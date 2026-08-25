using System.Collections.Generic;

using Utf8Json;
using Webserver;
using Webserver.Permissions;
using Webserver.WebAPI;

namespace Xylona.SevenDaysToDie {
	public sealed class GetLandClaims : AbsRestApi {
		private static readonly byte[] jsonKeyClaimSize = JsonWriter.GetEncodedPropertyNameWithBeginObject ("claimsize");
		private static readonly byte[] jsonKeyClaimOwners = JsonWriter.GetEncodedPropertyNameWithPrefixValueSeparator ("claimowners");
		private static readonly byte[] jsonKeySteamId = JsonWriter.GetEncodedPropertyNameWithBeginObject ("steamid");
		private static readonly byte[] jsonKeyClaimActive = JsonWriter.GetEncodedPropertyNameWithPrefixValueSeparator ("claimactive");
		private static readonly byte[] jsonKeyPlayerName = JsonWriter.GetEncodedPropertyNameWithPrefixValueSeparator ("playername");
		private static readonly byte[] jsonKeyClaims = JsonWriter.GetEncodedPropertyNameWithPrefixValueSeparator ("claims");
		private static readonly byte[] jsonKeyX = JsonWriter.GetEncodedPropertyNameWithBeginObject ("x");
		private static readonly byte[] jsonKeyY = JsonWriter.GetEncodedPropertyNameWithPrefixValueSeparator ("y");
		private static readonly byte[] jsonKeyZ = JsonWriter.GetEncodedPropertyNameWithPrefixValueSeparator ("z");

#if SEVEN_DAYS_V3
		public override void HandleRestGet (RequestContext _context) {
#else
		protected override void HandleRestGet (RequestContext _context) {
#endif
			Dictionary<PersistentPlayerData, List<Vector3i>> claimsByOwner = GetClaimsByOwner ();
			GameManager manager = GameManager.Instance;
			World world = manager?.World;

			PrepareEnvelopedResult (out JsonWriter writer);
			writer.WriteRaw (jsonKeyClaimSize);
			writer.WriteInt32 (GamePrefs.GetInt (EnumGamePrefs.LandClaimSize));
			writer.WriteRaw (jsonKeyClaimOwners);
			writer.WriteBeginArray ();

			bool firstOwner = true;
			foreach (KeyValuePair<PersistentPlayerData, List<Vector3i>> entry in claimsByOwner) {
				PersistentPlayerData owner = entry.Key;
				string ownerId = owner.PrimaryId?.CombinedString;
				if (string.IsNullOrEmpty (ownerId)) {
					continue;
				}

				if (!firstOwner) {
					writer.WriteValueSeparator ();
				}
				firstOwner = false;

				writer.WriteRaw (jsonKeySteamId);
				writer.WriteString (ownerId);
				writer.WriteRaw (jsonKeyClaimActive);
				writer.WriteBoolean (world != null && world.IsLandProtectionValidForPlayer (owner));
				writer.WriteRaw (jsonKeyPlayerName);
				writer.WriteString (owner.PlayerName?.SafeDisplayName ?? string.Empty);
				writer.WriteRaw (jsonKeyClaims);
				writer.WriteBeginArray ();

				bool firstClaim = true;
				foreach (Vector3i claim in entry.Value) {
					if (!firstClaim) {
						writer.WriteValueSeparator ();
					}
					firstClaim = false;

					writer.WriteRaw (jsonKeyX);
					writer.WriteInt32 (claim.x);
					writer.WriteRaw (jsonKeyY);
					writer.WriteInt32 (claim.y);
					writer.WriteRaw (jsonKeyZ);
					writer.WriteInt32 (claim.z);
					writer.WriteEndObject ();
				}

				writer.WriteEndArray ();
				writer.WriteEndObject ();
			}

			writer.WriteEndArray ();
			writer.WriteEndObject ();
			SendEnvelopedResult (_context, ref writer);
		}

		private static Dictionary<PersistentPlayerData, List<Vector3i>> GetClaimsByOwner () {
			Dictionary<PersistentPlayerData, List<Vector3i>> result = new Dictionary<PersistentPlayerData, List<Vector3i>> ();
			PersistentPlayerList players = GameManager.Instance?.GetPersistentPlayerList ();
			Dictionary<Vector3i, PersistentPlayerData> claims = players?.m_lpBlockMap;
			if (claims == null) {
				return result;
			}

			foreach (KeyValuePair<Vector3i, PersistentPlayerData> claim in claims) {
				if (claim.Value == null) {
					continue;
				}

				if (!result.TryGetValue (claim.Value, out List<Vector3i> positions)) {
					positions = new List<Vector3i> ();
					result.Add (claim.Value, positions);
				}
				positions.Add (claim.Key);
			}

			return result;
		}

		public override int[] DefaultMethodPermissionLevels () => new[] {
			AdminWebModules.MethodLevelNotSupported,
			AdminWebModules.MethodLevelInheritGlobal,
			AdminWebModules.MethodLevelNotSupported,
			AdminWebModules.MethodLevelNotSupported,
			AdminWebModules.MethodLevelNotSupported
		};

		public override int DefaultPermissionLevel () => AdminWebModules.PermissionLevelUser;
	}
}
