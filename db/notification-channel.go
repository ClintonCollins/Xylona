package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// encryptConfig encrypts a notification channel config value if an encryption
// key is configured. Returns the original value unchanged when no key is set.
func (c *Connection) encryptConfig(plaintext string) (string, error) {
	if len(c.encryptionKey) == 0 {
		return plaintext, nil
	}
	encrypted, errEncrypt := xycrypt.Encrypt(c.encryptionKey, plaintext)
	if errEncrypt != nil {
		return "", fmt.Errorf("encrypt config: %w", errEncrypt)
	}
	return encrypted, nil
}

// decryptConfig decrypts a notification channel config value if an encryption
// key is configured. Returns the stored value unchanged when no key is set.
func (c *Connection) decryptConfig(ciphertext string) (string, error) {
	if len(c.encryptionKey) == 0 {
		return ciphertext, nil
	}
	decrypted, errDecrypt := xycrypt.Decrypt(c.encryptionKey, ciphertext)
	if errDecrypt != nil {
		return "", fmt.Errorf("decrypt config: %w", errDecrypt)
	}
	return decrypted, nil
}

// InsertNotificationChannel creates a new notification channel record. The
// config is encrypted at rest when an encryption key is configured. The
// returned record has its config in plaintext.
func (c *Connection) InsertNotificationChannel(userID, name, channelType, config string, enabled bool) (*models.NotificationChannel, error) {
	encryptedConfig, errEncrypt := c.encryptConfig(config)
	if errEncrypt != nil {
		log.Error().Err(errEncrypt).Msg("Error encrypting notification channel config")
		return nil, errEncrypt
	}

	enabledInt := int64(0)
	if enabled {
		enabledInt = 1
	}

	now := time.Now().UTC()
	id := uuid.New().String()

	setter := &models.NotificationChannelSetter{
		ID:          omit.From(id),
		UserID:      omit.From(userID),
		Name:        omit.From(name),
		ChannelType: omit.From(channelType),
		Config:      omit.From(encryptedConfig),
		Enabled:     omit.From(enabledInt),
		CreatedAt:   omit.From(now),
		UpdatedAt:   omit.From(now),
	}

	channel, errInsert := models.NotificationChannels.Insert(setter).One(c.ctx, c.DB)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Error inserting notification channel")
		return nil, errInsert
	}

	// Return the channel with the plaintext config for the caller.
	channel.Config = config
	return channel, nil
}

// GetNotificationChannelByID fetches a single notification channel by ID,
// decrypting its config before returning.
func (c *Connection) GetNotificationChannelByID(id string) (*models.NotificationChannel, error) {
	channel, errGet := models.NotificationChannels.Query(models.SelectWhere.NotificationChannels.ID.EQ(id)).One(c.ctx, c.DB)
	if errGet != nil {
		if !errors.Is(errGet, sql.ErrNoRows) {
			log.Error().Err(errGet).Str("notification_channel_id", id).Msg("Error querying notification channel by ID")
		}
		return nil, errGet
	}

	decrypted, errDecrypt := c.decryptConfig(channel.Config)
	if errDecrypt != nil {
		log.Error().Err(errDecrypt).Str("notification_channel_id", id).Msg("Error decrypting notification channel config")
		return nil, errDecrypt
	}
	channel.Config = decrypted

	return channel, nil
}

// GetNotificationChannelsByUserID fetches all notification channels belonging
// to a user, decrypting each config before returning.
func (c *Connection) GetNotificationChannelsByUserID(userID string) ([]*models.NotificationChannel, error) {
	channels, errGet := models.NotificationChannels.Query(models.SelectWhere.NotificationChannels.UserID.EQ(userID)).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Str("user_id", userID).Msg("Error querying notification channels by user ID")
		return nil, errGet
	}

	for _, ch := range channels {
		decrypted, errDecrypt := c.decryptConfig(ch.Config)
		if errDecrypt != nil {
			log.Error().Err(errDecrypt).Str("notification_channel_id", ch.ID).Msg("Error decrypting notification channel config")
			return nil, errDecrypt
		}
		ch.Config = decrypted
	}

	return channels, nil
}

// UpdateNotificationChannel updates the name, config, and enabled state of a
// notification channel identified by id and userID. The config is encrypted
// at rest when an encryption key is configured.
func (c *Connection) UpdateNotificationChannel(id, userID, name, config string, enabled bool) error {
	encryptedConfig, errEncrypt := c.encryptConfig(config)
	if errEncrypt != nil {
		log.Error().Err(errEncrypt).Str("notification_channel_id", id).Msg("Error encrypting notification channel config for update")
		return errEncrypt
	}

	enabledInt := int64(0)
	if enabled {
		enabledInt = 1
	}

	now := time.Now().UTC()

	setter := &models.NotificationChannelSetter{
		Name:      omit.From(name),
		Config:    omit.From(encryptedConfig),
		Enabled:   omit.From(enabledInt),
		UpdatedAt: omit.From(now),
	}

	_, errUpdate := models.NotificationChannels.Update(
		setter.UpdateMod(),
		models.UpdateWhere.NotificationChannels.ID.EQ(id),
		models.UpdateWhere.NotificationChannels.UserID.EQ(userID),
	).Exec(c.ctx, c.DB)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("notification_channel_id", id).Msg("Error updating notification channel")
		return errUpdate
	}

	return nil
}

// DeleteNotificationChannel deletes the notification channel identified by id
// and userID. Scoping the delete to userID prevents cross-user deletion.
func (c *Connection) DeleteNotificationChannel(id, userID string) error {
	_, errDelete := models.NotificationChannels.Delete(
		models.DeleteWhere.NotificationChannels.ID.EQ(id),
		models.DeleteWhere.NotificationChannels.UserID.EQ(userID),
	).Exec(c.ctx, c.DB)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("notification_channel_id", id).Msg("Error deleting notification channel")
		return errDelete
	}
	return nil
}
