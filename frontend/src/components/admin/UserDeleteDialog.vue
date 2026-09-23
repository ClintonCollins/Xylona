<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="user-delete-dialog-title"
    backdrop-filter="brightness(15%)"
    persistent>
    <q-card class="user-delete-dialog">
      <q-card-section>
        <div id="user-delete-dialog-title" class="text-h6 text-negative">Delete User</div>
      </q-card-section>
      <q-card-section class="user-delete-body">
        <div v-if="loadingImpact" class="user-delete-loading" role="status">
          <q-spinner color="primary" size="1.25rem" />
          Checking what {{ user?.userName }} owns…
        </div>

        <q-banner v-else-if="impactError" class="xy-banner-negative" dense role="alert">
          Could not check what {{ user?.userName }} owns. {{ impactError }}
        </q-banner>

        <template v-else-if="blocked">
          <p>
            <strong>{{ user?.userName }}</strong> can't be deleted yet.
          </p>
          <div v-if="impact?.ownedGameServers.length" class="user-delete-group">
            <p>Change the owner of these game servers in their Settings first:</p>
            <ul>
              <li v-for="gameServer in impact.ownedGameServers" :key="gameServer.id">
                <router-link :to="`/game-servers/${gameServer.id}/settings`">
                  {{ gameServer.name }}
                </router-link>
              </li>
            </ul>
          </div>
          <div v-if="impact?.grantsGiven.length" class="user-delete-group">
            <p>Remove the access they gave other users from each server's Access tab first:</p>
            <ul>
              <li
                v-for="grant in impact.grantsGiven"
                :key="`${grant.gameServerId}:${grant.userName}`">
                {{ grant.userName }} on
                <router-link
                  v-if="grant.gameServerId"
                  :to="`/game-servers/${grant.gameServerId}/access`">
                  {{ grant.gameServerName }}
                </router-link>
                <template v-else>every game server</template>
              </li>
            </ul>
          </div>
        </template>

        <template v-else>
          <p>
            Are you sure you want to delete <strong>{{ user?.userName }}</strong
            >? <strong>This action cannot be undone.</strong>
          </p>
          <div
            v-if="impact?.schedules.length"
            class="user-delete-group xy-banner-warning user-delete-schedules">
            <p>
              {{ impact.schedules.length === 1 ? 'This schedule' : 'These schedules' }} they created
              will be deleted too:
            </p>
            <ul>
              <li
                v-for="schedule in impact.schedules"
                :key="`${schedule.gameServerId}:${schedule.name}`">
                {{ schedule.gameServerName }} &middot; {{ schedule.name }}
              </li>
            </ul>
          </div>
        </template>

        <q-banner v-if="deleteError" class="xy-banner-negative q-mt-md" dense role="alert">
          {{ deleteError }}
        </q-banner>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn :disable="deleting" flat label="Cancel" @click="showDialog = false" />
        <q-btn
          v-if="!blocked"
          :disable="loadingImpact || impactError !== ''"
          :loading="deleting"
          color="negative"
          label="Delete"
          unelevated
          @click="deleteUser" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { computed, PropType, ref, watch } from 'vue'
import { GetXylonaClient } from '@/utils/shared'
import { connectErrorMessage } from '@/api/connect-errors'
import { notifySuccess } from '@/api/notifications'
import {
  DeleteUserRequestSchema,
  GetUserDeletionImpactRequestSchema,
  type GetUserDeletionImpactResponse,
  User,
} from '@/proto/xylona_pb'

const props = defineProps({
  user: {
    type: Object as PropType<User | null>,
    default: null,
  },
})

const emit = defineEmits<{
  submit: [error: boolean]
}>()

const showDialog = defineModel<boolean>('showDialog', {
  default: false,
})

const impact = ref<GetUserDeletionImpactResponse | null>(null)
const loadingImpact = ref(false)
const impactError = ref('')
const deleting = ref(false)
const deleteError = ref('')

// Owned servers and access given to others fail the delete, so the dialog
// names them instead of offering a Delete that cannot succeed.
const blocked = computed(
  () => (impact.value?.ownedGameServers.length ?? 0) + (impact.value?.grantsGiven.length ?? 0) > 0,
)

watch(
  () => [showDialog.value, props.user?.id] as const,
  ([open, userID]) => {
    if (open && userID) {
      void loadImpact(userID)
    }
  },
  { immediate: true },
)

async function loadImpact(userID: string) {
  impact.value = null
  impactError.value = ''
  deleteError.value = ''
  loadingImpact.value = true
  try {
    const response = await GetXylonaClient().getUserDeletionImpact(
      create(GetUserDeletionImpactRequestSchema, { id: userID }),
    )
    if (props.user?.id === userID) {
      impact.value = response
    }
  } catch (unknownError: unknown) {
    impactError.value = connectErrorMessage(unknownError)
  } finally {
    loadingImpact.value = false
  }
}

async function deleteUser() {
  if (!props.user) {
    emit('submit', true)
    return
  }

  deleting.value = true
  deleteError.value = ''
  try {
    await GetXylonaClient().deleteUser(create(DeleteUserRequestSchema, { id: props.user.id }))
    notifySuccess(`${props.user.userName} deleted successfully`, { timeout: 5000 })
    showDialog.value = false
    emit('submit', false)
  } catch (unknownError: unknown) {
    deleteError.value = connectErrorMessage(unknownError)
    emit('submit', true)
  } finally {
    deleting.value = false
  }
}
</script>

<style scoped>
.user-delete-dialog {
  width: min(32rem, 100%);
}

.user-delete-body p {
  margin: 0;
}

.user-delete-loading {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  color: var(--xy-text-secondary);
}

.user-delete-group {
  margin-top: var(--xy-space-md);
}

.user-delete-group ul {
  margin: var(--xy-space-xs) 0 0;
  padding-left: var(--xy-space-lg);
}

.user-delete-group a {
  color: var(--xy-primary);
}

.user-delete-schedules {
  padding: var(--xy-space-sm) var(--xy-space-md);
}
</style>
