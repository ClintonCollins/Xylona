<template>
  <section aria-labelledby="dns-provider-heading" class="dns-provider-settings">
    <div class="xy-section-header q-mb-md">
      <h2 id="dns-provider-heading" class="text-h6 q-my-none">DNS Provider</h2>
      <p class="dns-provider-description">
        Authorize this controller to manage records in one existing authoritative zone.
      </p>
    </div>

    <q-skeleton v-if="loading" height="18rem" type="rect" />

    <template v-else>
      <div class="dns-status-strip" role="status">
        <q-icon
          :color="configured ? 'positive' : 'warning'"
          :name="configured ? 'check_circle' : 'warning'"
          size="1.4rem" />
        <div>
          <div class="dns-status-title">
            {{
              configured ? 'DNS provider connection is active' : 'DNS provider is not configured'
            }}
          </div>
          <div class="dns-status-detail">
            <template v-if="configured && activeConnection">
              {{ providerLabel(activeConnection.provider) }} · {{ activeConnection.zoneName }} ·
              <span class="font-mono">{{ activeConnection.zoneId }}</span>
            </template>
            <template v-else>Choose a provider and test an existing zone below.</template>
          </div>
        </div>
      </div>

      <q-banner v-if="loadError" class="dns-error q-mt-md" role="alert" rounded>
        {{ loadError }}
      </q-banner>

      <q-btn-toggle
        v-model="provider"
        :options="providerOptions"
        aria-label="DNS provider"
        class="provider-toggle q-mt-lg"
        no-caps
        outline
        spread
        toggle-color="primary"
        unelevated
        @update:model-value="selectProvider" />

      <div class="dns-provider-panel q-mt-md">
        <div v-if="provider === DNSProviderKind.DNS_PROVIDER_KIND_CLOUDFLARE">
          <div class="provider-title">
            <h3 class="text-subtitle1 q-my-none">Cloudflare</h3>
            <span class="permission-help">
              <q-btn
                aria-label="Cloudflare permission requirements"
                class="permission-help-trigger"
                data-testid="cloudflare-permission-help"
                dense
                flat
                icon="info_outline"
                round
                size="sm">
                <q-tooltip>Permission requirements</q-tooltip>
              </q-btn>
              <q-menu anchor="bottom left" :offset="[0, 8]" self="top left">
                <div class="permission-help-content" data-testid="cloudflare-permission-guidance">
                  <div class="permission-help-title">Cloudflare permissions</div>
                  <p class="permission-help-intro">Scope the token to the selected zone.</p>
                  <dl class="permission-help-list">
                    <div>
                      <dt class="font-mono">Zone Read</dt>
                      <dd>List and verify the zone.</dd>
                    </div>
                    <div>
                      <dt class="font-mono">DNS Write</dt>
                      <dd>Read, create, and update A and AAAA records.</dd>
                    </div>
                  </dl>
                </div>
              </q-menu>
            </span>
          </div>
          <p class="provider-copy">
            Use a scoped API token for the selected zone. Records created by Xylona are always
            DNS-only.
          </p>
          <q-input
            v-model="cloudflareApiToken"
            :hint="secretHint('API token')"
            aria-label="Cloudflare API token"
            autocomplete="new-password"
            data-testid="cloudflare-api-token"
            dense
            label="API Token"
            outlined
            type="password" />
        </div>

        <div v-else>
          <div class="provider-title">
            <h3 class="text-subtitle1 q-my-none">Amazon Route 53</h3>
            <span class="permission-help">
              <q-btn
                aria-label="Route 53 permission requirements"
                class="permission-help-trigger"
                data-testid="route53-permission-help"
                dense
                flat
                icon="info_outline"
                round
                size="sm">
                <q-tooltip>Permission requirements</q-tooltip>
              </q-btn>
              <q-menu anchor="bottom left" :offset="[0, 8]" self="top left">
                <div class="permission-help-content" data-testid="route53-permission-guidance">
                  <div class="permission-help-title">Route 53 permissions</div>
                  <p class="permission-help-intro">Apply these to the selected hosted zone.</p>
                  <dl class="permission-help-list">
                    <div>
                      <dt class="font-mono">route53:GetHostedZone</dt>
                      <dd>Verify the exact hosted zone.</dd>
                    </div>
                    <div>
                      <dt class="font-mono">route53:ListResourceRecordSets</dt>
                      <dd>Read the current record.</dd>
                    </div>
                    <div>
                      <dt class="font-mono">route53:ChangeResourceRecordSets</dt>
                      <dd>Create and update A and AAAA records.</dd>
                    </div>
                  </dl>
                  <p class="permission-help-note">
                    <span class="font-mono">route53:ListHostedZones</span> is needed only by List
                    zones and must allow all resources (<span class="font-mono">Resource: *</span>).
                    Exact zone entry does not need it.
                  </p>
                </div>
              </q-menu>
            </span>
          </div>
          <p class="provider-copy">
            Use the controller runtime credential chain or enter access keys. Stored access keys are
            encrypted and never returned.
          </p>
          <q-option-group
            v-model="credentialMode"
            :options="route53CredentialOptions"
            aria-label="Route 53 credential source"
            color="primary"
            inline />

          <div
            v-if="credentialMode === DNSCredentialMode.DNS_CREDENTIAL_MODE_AWS_ACCESS_KEY"
            class="row q-col-gutter-md q-mt-sm">
            <div class="col-12 col-md-6">
              <q-input
                v-model="awsAccessKeyId"
                :hint="secretHint('Access key ID')"
                aria-label="AWS access key ID"
                autocomplete="new-password"
                dense
                label="Access Key ID"
                outlined
                type="password" />
            </div>
            <div class="col-12 col-md-6">
              <q-input
                v-model="awsSecretAccessKey"
                :hint="secretHint('Secret access key')"
                aria-label="AWS secret access key"
                autocomplete="new-password"
                dense
                label="Secret Access Key"
                outlined
                type="password" />
            </div>
          </div>
          <q-banner v-else class="runtime-note q-mt-md" rounded>
            The controller will use the AWS SDK runtime credential chain when testing and managing
            records.
          </q-banner>
        </div>

        <div class="zone-heading q-mt-lg">
          <div>
            <h3 class="text-subtitle1 q-my-none">Authoritative zone</h3>
            <p class="provider-copy">
              List accessible zones, or enter the exact provider zone name and ID below.
            </p>
          </div>
          <q-btn
            :disable="!credentialsReady"
            :loading="listingZones"
            icon="refresh"
            label="List zones"
            data-testid="list-dns-zones"
            no-caps
            outline
            @click="loadZones" />
        </div>

        <q-banner v-if="zoneListError" class="dns-error q-mb-md" role="alert" rounded>
          {{ zoneListError }} Enter the exact zone name and ID instead.
        </q-banner>

        <q-select
          v-if="zones.length > 0"
          v-model="selectedZone"
          :options="zones"
          class="q-mb-md"
          dense
          label="Accessible Zones"
          option-label="name"
          outlined
          @update:model-value="selectZone" />

        <div class="row q-col-gutter-md">
          <div class="col-12 col-md-6">
            <q-input
              v-model="zoneName"
              aria-label="Authoritative zone name"
              data-testid="dns-zone-name"
              dense
              label="Exact Zone Name"
              outlined
              placeholder="example.com" />
          </div>
          <div class="col-12 col-md-6">
            <q-input
              v-model="zoneId"
              aria-label="Provider zone ID"
              data-testid="dns-zone-id"
              dense
              input-class="font-mono"
              label="Exact Zone ID"
              outlined />
          </div>
        </div>

        <q-banner v-if="activationError" class="dns-error q-mt-md" role="alert" rounded>
          {{ activationError }}
        </q-banner>

        <div class="provider-actions q-mt-lg">
          <q-btn
            :disable="!formReady"
            :loading="activating"
            color="primary"
            icon="verified"
            label="Test and activate"
            data-testid="activate-dns-provider"
            no-caps
            @click="activate" />
          <span class="action-note">Failed tests leave the active connection unchanged.</span>
        </div>
      </div>
    </template>
  </section>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, onMounted, ref } from 'vue'

import {
  DNSCredentialMode,
  type DNSProviderConnection,
  DNSProviderConnectionInputSchema,
  DNSProviderKind,
  type DNSProviderZone,
  GetDNSProviderConnectionRequestSchema,
  ListDNSProviderZonesRequestSchema,
  SetDNSProviderConnectionRequestSchema,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const $q = useQuasar()
const loading = ref(true)
const listingZones = ref(false)
const activating = ref(false)
const configured = ref(false)
const activeConnection = ref<DNSProviderConnection>()
const provider = ref(DNSProviderKind.DNS_PROVIDER_KIND_CLOUDFLARE)
const credentialMode = ref(DNSCredentialMode.DNS_CREDENTIAL_MODE_CLOUDFLARE_API_TOKEN)
const cloudflareApiToken = ref('')
const awsAccessKeyId = ref('')
const awsSecretAccessKey = ref('')
const zoneName = ref('')
const zoneId = ref('')
const zones = ref<DNSProviderZone[]>([])
const selectedZone = ref<DNSProviderZone>()
const loadError = ref('')
const zoneListError = ref('')
const activationError = ref('')

const providerOptions = [
  {
    label: 'Cloudflare',
    value: DNSProviderKind.DNS_PROVIDER_KIND_CLOUDFLARE,
    icon: 'cloud',
  },
  {
    label: 'Amazon Route 53',
    value: DNSProviderKind.DNS_PROVIDER_KIND_ROUTE53,
    icon: 'route',
  },
]
const route53CredentialOptions = [
  {
    label: 'Runtime credential chain',
    value: DNSCredentialMode.DNS_CREDENTIAL_MODE_AWS_RUNTIME,
  },
  {
    label: 'Access keys',
    value: DNSCredentialMode.DNS_CREDENTIAL_MODE_AWS_ACCESS_KEY,
  },
]

const canPreserveCredentials = computed(
  () =>
    activeConnection.value?.credentialsConfigured === true &&
    activeConnection.value.provider === provider.value &&
    activeConnection.value.credentialMode === credentialMode.value,
)
const credentialsReady = computed(() => {
  if (credentialMode.value === DNSCredentialMode.DNS_CREDENTIAL_MODE_AWS_RUNTIME) return true
  if (credentialMode.value === DNSCredentialMode.DNS_CREDENTIAL_MODE_CLOUDFLARE_API_TOKEN) {
    return cloudflareApiToken.value.trim() !== '' || canPreserveCredentials.value
  }
  const accessKeyIdPresent = awsAccessKeyId.value.trim() !== ''
  const secretAccessKeyPresent = awsSecretAccessKey.value.trim() !== ''
  return (
    (accessKeyIdPresent && secretAccessKeyPresent) ||
    (!accessKeyIdPresent && !secretAccessKeyPresent && canPreserveCredentials.value)
  )
})
const formReady = computed(
  () => credentialsReady.value && zoneName.value.trim() !== '' && zoneId.value.trim() !== '',
)
function providerLabel(value: DNSProviderKind): string {
  return value === DNSProviderKind.DNS_PROVIDER_KIND_CLOUDFLARE ? 'Cloudflare' : 'Amazon Route 53'
}

function selectProvider(nextProvider: DNSProviderKind): void {
  credentialMode.value =
    nextProvider === DNSProviderKind.DNS_PROVIDER_KIND_CLOUDFLARE
      ? DNSCredentialMode.DNS_CREDENTIAL_MODE_CLOUDFLARE_API_TOKEN
      : DNSCredentialMode.DNS_CREDENTIAL_MODE_AWS_RUNTIME
  zones.value = []
  selectedZone.value = undefined
  zoneListError.value = ''
  activationError.value = ''
}

function candidate() {
  return create(DNSProviderConnectionInputSchema, {
    provider: provider.value,
    zoneName: zoneName.value.trim(),
    zoneId: zoneId.value.trim(),
    credentialMode: credentialMode.value,
    cloudflareApiToken: cloudflareApiToken.value,
    awsAccessKeyId: awsAccessKeyId.value,
    awsSecretAccessKey: awsSecretAccessKey.value,
  })
}

function applyConnection(connection: DNSProviderConnection): void {
  configured.value = true
  activeConnection.value = connection
  provider.value = connection.provider
  credentialMode.value = connection.credentialMode
  zoneName.value = connection.zoneName
  zoneId.value = connection.zoneId
  clearSecrets()
}

function clearSecrets(): void {
  cloudflareApiToken.value = ''
  awsAccessKeyId.value = ''
  awsSecretAccessKey.value = ''
}

function secretHint(label: string): string {
  return canPreserveCredentials.value
    ? `${label} is stored. Leave blank to keep it.`
    : 'Write-only and stored encrypted by the controller.'
}

function selectZone(zone: DNSProviderZone | null): void {
  if (!zone) return
  zoneName.value = zone.name
  zoneId.value = zone.id
}

async function loadConnection(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const response = await GetXylonaClient().getDNSProviderConnection(
      create(GetDNSProviderConnectionRequestSchema),
    )
    configured.value = response.configured
    if (response.connection) applyConnection(response.connection)
  } catch (unknownError: unknown) {
    loadError.value = sanitizedError(unknownError)
  } finally {
    loading.value = false
  }
}

async function loadZones(): Promise<void> {
  if (!credentialsReady.value) return
  listingZones.value = true
  zoneListError.value = ''
  try {
    const response = await GetXylonaClient().listDNSProviderZones(
      create(ListDNSProviderZonesRequestSchema, { candidate: candidate() }),
    )
    zones.value = response.zones
    const activeZone = zones.value.find(
      (zone) => zone.id === zoneId.value && zone.name === zoneName.value,
    )
    selectedZone.value = activeZone
  } catch (unknownError: unknown) {
    zones.value = []
    selectedZone.value = undefined
    zoneListError.value = sanitizedError(unknownError)
  } finally {
    listingZones.value = false
  }
}

async function activate(): Promise<void> {
  if (!formReady.value) return
  activating.value = true
  activationError.value = ''
  try {
    const response = await GetXylonaClient().setDNSProviderConnection(
      create(SetDNSProviderConnectionRequestSchema, { candidate: candidate() }),
    )
    if (!response.connection) throw new Error('The activated DNS connection response was empty.')
    applyConnection(response.connection)
    $q.notify({
      type: 'positive',
      message: 'DNS provider connection tested and activated.',
    })
  } catch (unknownError: unknown) {
    activationError.value = sanitizedError(unknownError)
  } finally {
    activating.value = false
  }
}

function sanitizedError(error: unknown): string {
  return ConnectErrorToString(ConnectError.from(error))
}

onMounted(loadConnection)
</script>

<style scoped>
.dns-provider-settings {
  max-width: 960px;
  margin-top: var(--xy-space-xl);
}

.dns-provider-description,
.provider-copy,
.action-note {
  max-width: 72ch;
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.dns-status-strip,
.dns-provider-panel {
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.dns-status-strip {
  display: flex;
  align-items: center;
  gap: var(--xy-space-base);
  padding: var(--xy-space-md);
}

.dns-status-title {
  color: var(--xy-text-primary);
  font-weight: 600;
}

.dns-status-detail {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
  overflow-wrap: anywhere;
}

.dns-provider-panel {
  padding: var(--xy-space-md);
}

.provider-title {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
}

.zone-heading,
.provider-actions {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.provider-actions {
  align-items: center;
  justify-content: flex-start;
}

.permission-help {
  display: inline-flex;
}

.permission-help-trigger {
  color: var(--xy-text-secondary);
}

.permission-help-content {
  width: min(27rem, calc(100vw - var(--xy-space-xl)));
  padding: var(--xy-space-md);
}

.permission-help-title {
  color: var(--xy-text-primary);
  font-weight: 600;
}

.permission-help-intro,
.permission-help-list dd,
.permission-help-note {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.permission-help-intro {
  margin: var(--xy-space-xs) 0 0;
}

.permission-help-list {
  display: grid;
  gap: var(--xy-space-sm);
  margin: var(--xy-space-base) 0 0;
}

.permission-help-list dt {
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-xs);
  overflow-wrap: anywhere;
}

.permission-help-list dd {
  margin: var(--xy-space-2xs) 0 0;
}

.permission-help-note {
  margin: var(--xy-space-base) 0 0;
  padding-top: var(--xy-space-base);
  border-top: 1px solid var(--xy-border);
}

.runtime-note {
  color: var(--xy-text-secondary);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
}

.dns-error {
  color: var(--xy-text-primary);
  background: var(--xy-error-bg-faint);
  border: 1px solid var(--xy-error-border);
}

@media (max-width: 599px) {
  .zone-heading,
  .provider-actions {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
