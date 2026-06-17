<template>
  <div class="app">

    <!-- ── Left sidebar ───────────────────────────────────────────────── -->
    <aside class="sidebar">
      <div class="server-header">
        <span class="server-name">{{ activeServer?.name || 'Roundtable' }}</span>
        <button class="icon-btn" title="Settings" @click="openSettings('servers')">⚙</button>
      </div>

      <div class="section-label">
        <span>Channels</span>
        <button class="icon-btn" title="New channel" @click="showNewRoom = !showNewRoom">+</button>
      </div>

      <div v-if="showNewRoom" class="new-room-row">
        <input
          ref="newRoomInput"
          v-model="newRoomName"
          placeholder="channel-name"
          @keyup.enter="createRoom"
          @keyup.esc="showNewRoom = false"
        />
        <button @click="createRoom">Create</button>
      </div>

      <ul class="channel-list">
        <li
          v-for="room in rooms"
          :key="room.id"
          class="channel-item"
          :class="{ active: activeRoomID === room.id }"
          @click="joinRoom(room)"
        >
          <span class="hash">#</span>
          <span class="ch-name">{{ room.name }}</span>
          <span class="ch-count">{{ room.member_count }}</span>
          <button
            class="ch-delete"
            title="Delete channel"
            @click.stop="deleteRoom(room)"
          >🗑</button>
        </li>
      </ul>

      <div class="sidebar-spacer" />

      <div class="user-bar">
        <button class="icon-btn" title="Voice & Audio settings" @click="openSettings('audio')">🎙</button>
        <span class="user-name">{{ displayName || 'Me' }}</span>
        <button
          class="mute-btn"
          :class="{ muted }"
          title="Toggle mute"
          :disabled="!connected"
          @click="toggleMute"
        >{{ muted ? '🔇' : '🎤' }}</button>
      </div>
    </aside>

    <!-- ── Main content ──────────────────────────────────────────────── -->
    <main class="main">
      <div v-if="activeRoomID" class="room-view">
        <div class="room-header">
          <span class="hash">#</span>
          <span>{{ activeRoomName }}</span>
          <button v-if="connected" class="leave-btn" @click="leaveRoom">Leave</button>
        </div>
        <div class="members-grid">
          <div v-if="roomMembers.length === 0" class="empty-hint">No one here yet — join to be the first</div>
          <div v-for="m in roomMembers" :key="m" class="member-card">
            <div class="member-avatar">{{ initials(m) }}</div>
            <div class="member-name">{{ m }}</div>
          </div>
        </div>
      </div>
      <div v-else class="welcome">
        <div class="welcome-icon">🔊</div>
        <div class="welcome-title">{{ activeServer ? activeServer.name : 'No server connected' }}</div>
        <div class="welcome-sub">{{ activeServer ? 'Select a channel to join' : 'Add a server in Settings (⚙)' }}</div>
      </div>
    </main>

    <!-- ── Audio bar ─────────────────────────────────────────────────── -->
    <footer class="audio-bar">
      <div class="level-track" title="Microphone level">
        <div class="level-fill" :style="{ width: micLevelPct + '%' }" :class="levelClass" />
      </div>
      <span class="status-dot" :class="connected ? 'online' : 'offline'" />
      <span class="status-text">{{ connected ? `#${activeRoomName}` : 'Not in a channel' }}</span>
      <div v-if="error" class="error-pill">{{ error }}</div>
    </footer>

    <!-- ── Settings overlay ──────────────────────────────────────────── -->
    <Transition name="fade">
      <div v-if="settingsOpen" class="overlay" @click.self="settingsOpen = false">
        <div class="settings-modal">

          <nav class="settings-nav">
            <div class="settings-nav-group">
              <div class="settings-nav-label">User</div>
              <button :class="{ active: settingsTab === 'account' }" @click="settingsTab = 'account'">Account</button>
              <button :class="{ active: settingsTab === 'audio' }" @click="settingsTab = 'audio'">Voice &amp; Audio</button>
            </div>
            <div class="settings-nav-group">
              <div class="settings-nav-label">App</div>
              <button :class="{ active: settingsTab === 'servers' }" @click="settingsTab = 'servers'">Servers</button>
            </div>
            <button class="settings-close" @click="settingsOpen = false">✕ ESC</button>
          </nav>

          <section class="settings-body">
            <div v-if="settingsError" class="error-pill settings-error" @click="settingsError = ''">{{ settingsError }}</div>

            <!-- Account -->
            <div v-if="settingsTab === 'account'" class="settings-section">
              <h2>Account</h2>
              <div class="field">
                <label>Display name</label>
                <input v-model="displayName" placeholder="Your name" @change="saveConfig" />
                <div class="field-hint">Shown to others in voice channels</div>
              </div>
            </div>

            <!-- Voice & Audio -->
            <div v-if="settingsTab === 'audio'" class="settings-section">
              <h2>Voice &amp; Audio</h2>

              <div class="field">
                <label>Input device (microphone)</label>
                <select v-model="selectedCapture" @change="saveConfig">
                  <option value="">System default</option>
                  <option v-for="d in devices.capture" :key="d.name" :value="d.name">{{ d.name }}</option>
                </select>
              </div>

              <div class="field">
                <label>Output device (speaker)</label>
                <select v-model="selectedPlayback" @change="saveConfig">
                  <option value="">System default</option>
                  <option v-for="d in devices.playback" :key="d.name" :value="d.name">{{ d.name }}</option>
                </select>
              </div>

              <div class="field">
                <label>Input volume <span class="value-badge">{{ micGainDB > 0 ? '+' : '' }}{{ micGainDB }} dB</span></label>
                <input type="range" min="-20" max="20" step="1" v-model.number="micGainDB" @change="onMicGainChange" />
              </div>

              <div class="field">
                <label>
                  Voice activation
                  <span class="value-badge">{{ voiceActivationDB }} dB</span>
                  <span class="field-hint inline">Signals below this threshold are suppressed</span>
                </label>
                <input type="range" min="-70" max="-20" step="1" v-model.number="voiceActivationDB" @change="onVAChange" />
              </div>

              <div class="field toggle-field">
                <label>Noise suppression</label>
                <div class="toggle-wrap">
                  <button
                    class="toggle"
                    :class="{ on: noiseSuppressionEnabled }"
                    :disabled="!noiseSuppressionAvailable"
                    @click="toggleNoiseSuppression"
                  >{{ noiseSuppressionEnabled ? 'On' : 'Off' }}</button>
                  <span class="field-hint">{{ noiseSuppressionAvailable ? 'Removes background noise in real time' : 'DeepFilterNet unavailable — check terminal for errors' }}</span>
                </div>
              </div>

              <div class="field toggle-field">
                <label>Loopback test</label>
                <div class="toggle-wrap">
                  <button
                    class="toggle"
                    :class="{ on: loopbackEnabled }"
                    @click="toggleLoopback"
                  >{{ loopbackEnabled ? 'On' : 'Off' }}</button>
                  <span class="field-hint">{{ connected ? 'Hear your own mic through the voice session' : 'Hear your own mic without joining a room' }}</span>
                </div>
              </div>

              <div class="field">
                <label>Live mic level</label>
                <div class="level-track preview-level">
                  <div class="level-fill" :style="{ width: micLevelPct + '%' }" :class="levelClass" />
                </div>
                <div class="field-hint">{{ micLevelDB.toFixed(1) }} dB</div>
              </div>
            </div>

            <!-- Servers -->
            <div v-if="settingsTab === 'servers'" class="settings-section">
              <h2>Servers</h2>

              <div class="server-list">
                <div
                  v-for="(srv, i) in servers"
                  :key="i"
                  class="server-row"
                  :class="{ active: activeServer?.url === srv.http_url }"
                >
                  <div class="srv-info">
                    <div class="srv-name">{{ srv.name || srv.http_url }}</div>
                    <div class="srv-url">{{ srv.http_url }}</div>
                  </div>
                  <button class="small-btn" @click="connectToServer(srv.http_url!)">
                    {{ activeServer?.url === srv.http_url ? '✓ Connected' : 'Connect' }}
                  </button>
                  <button class="small-btn danger" @click="removeServer(i)">Remove</button>
                </div>
                <div v-if="servers.length === 0" class="empty-hint">No servers added yet</div>
              </div>

              <div class="add-server-form">
                <h3>Add server</h3>
                <div class="field">
                  <label>Name (optional)</label>
                  <input v-model="newServerName" placeholder="My server" />
                </div>
                <div class="field">
                  <label>URL</label>
                  <input v-model="newServerURL" placeholder="http://localhost:1323" @keyup.enter="addServer" />
                </div>
                <button class="primary-btn" @click="addServer">Add &amp; connect</button>
              </div>
            </div>

          </section>
        </div>
      </div>
    </Transition>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { Rooms, Audio, Session, Config, Events, type Room, type AudioDevices, type ServerConfig } from './wails'

// ── State ─────────────────────────────────────────────────────────────────────

const rooms       = ref<Room[]>([])
const activeRoomID   = ref('')
const activeRoomName = ref('')
const roomMembers = ref<string[]>([])
const connected   = ref(false)
const muted       = ref(false)
const error       = ref('')

const settingsOpen = ref(false)
const settingsTab  = ref<'account' | 'audio' | 'servers'>('account')
const showNewRoom  = ref(false)
const newRoomName  = ref('')
const newRoomInput = ref<HTMLInputElement | null>(null)

// User & audio config
const displayName         = ref('Me')
const selectedCapture     = ref('')
const selectedPlayback    = ref('')
const micGainDB           = ref(0)
const voiceActivationDB   = ref(-42)
const noiseSuppressionEnabled   = ref(false)
const noiseSuppressionAvailable = ref(false)
const loopbackEnabled           = ref(false)
const settingsError             = ref('')
const micLevelDB          = ref(-70)

// Server config
const activeServer  = ref<{ name: string; url: string } | null>(null)
const servers       = ref<ServerConfig[]>([])
const newServerName = ref('')
const newServerURL  = ref('')

const devices = ref<AudioDevices>({ capture: [], playback: [] })

// ── Computed ─────────────────────────────────────────────────────────────────

const micLevelPct = computed(() => Math.max(0, Math.min(100, ((micLevelDB.value + 70) / 70) * 100)))
const levelClass  = computed(() => {
  if (micLevelDB.value > -12) return 'level-hot'
  if (micLevelDB.value > -30) return 'level-ok'
  return 'level-low'
})

// ── Intervals ─────────────────────────────────────────────────────────────────

let micPoll:  ReturnType<typeof setInterval> | null = null
let roomPoll: ReturnType<typeof setInterval> | null = null

// ── Lifecycle ─────────────────────────────────────────────────────────────────

onMounted(async () => {
  await loadConfig()
  await loadDevices()
  noiseSuppressionAvailable.value = await Session.isNoiseSuppressionAvailable().catch(() => false) as boolean

  // Wails v3 dispatches Go events as custom DOM events named "wails:event:<name>"
  window.addEventListener('wails:event:rooms:members', onMembersEvent as EventListener)

  micPoll = setInterval(async () => {
    if (connected.value) {
      micLevelDB.value = (await Session.getMicLevelDB()) ?? -70
    }
  }, 100)

  roomPoll = setInterval(async () => {
    if (activeServer.value) refreshRooms()
  }, 5000)
})

onUnmounted(() => {
  window.removeEventListener('wails:event:rooms:members', onMembersEvent as EventListener)
  if (micPoll)  clearInterval(micPoll)
  if (roomPoll) clearInterval(roomPoll)
  Events.unsubscribe()
  Audio.stopLoopback().catch(() => {})
  if (connected.value) Session.disconnect()
})

watch(showNewRoom, async (v) => {
  if (v) {
    await nextTick()
    newRoomInput.value?.focus()
  }
})

// ── Event handler ─────────────────────────────────────────────────────────────

function onMembersEvent(e: CustomEvent) {
  const data: { room_id: string; members: string[] } = e.detail
  if (data.room_id === activeRoomID.value) roomMembers.value = data.members ?? []
  const room = rooms.value.find(r => r.id === data.room_id)
  if (room) room.member_count = data.members?.length ?? 0
}

// ── Server actions ────────────────────────────────────────────────────────────

async function connectToServer(url: string) {
  error.value = ''
  try {
    const info = await Rooms.probe(url)
    activeServer.value = { name: info.name || url, url }
    if (!servers.value.find(s => s.http_url === url)) {
      servers.value.push({ name: info.name, http_url: url })
    } else {
      const s = servers.value.find(s => s.http_url === url)!
      if (!s.name && info.name) s.name = info.name
    }
    await refreshRooms()
    await Events.subscribe(url)
    await saveConfig()
    settingsOpen.value = false
  } catch (e: any) {
    error.value = e?.message ?? String(e)
  }
}

async function addServer() {
  const url = newServerURL.value.trim()
  if (!url) return
  newServerURL.value = ''
  newServerName.value = ''
  await connectToServer(url)
}

function removeServer(index: number) {
  const srv = servers.value[index]
  servers.value.splice(index, 1)
  if (activeServer.value?.url === srv.http_url) {
    activeServer.value = null
    rooms.value = []
    Events.unsubscribe()
  }
  saveConfig()
}

// ── Room actions ──────────────────────────────────────────────────────────────

async function refreshRooms() {
  if (!activeServer.value) return
  try {
    rooms.value = (await Rooms.list(activeServer.value.url)) ?? []
  } catch (e: any) {
    error.value = e?.message ?? String(e)
  }
}

async function createRoom() {
  if (!newRoomName.value.trim() || !activeServer.value) return
  try {
    const room = await Rooms.create(activeServer.value.url, newRoomName.value.trim())
    rooms.value.push(room)
    newRoomName.value = ''
    showNewRoom.value = false
  } catch (e: any) {
    error.value = e?.message ?? String(e)
  }
}

async function deleteRoom(room: Room) {
  if (!activeServer.value) return
  if (!confirm(`Delete #${room.name}?`)) return
  try {
    await Rooms.delete(activeServer.value.url, room.id)
    rooms.value = rooms.value.filter(r => r.id !== room.id)
    if (activeRoomID.value === room.id) leaveRoom()
  } catch (e: any) {
    error.value = e?.message ?? String(e)
  }
}

async function joinRoom(room: Room) {
  if (!activeServer.value) return
  if (connected.value) await leaveRoom()
  if (loopbackEnabled.value) {
    await Audio.stopLoopback().catch(() => {})
    loopbackEnabled.value = false
  }
  error.value = ''
  try {
    await Session.connect(
      activeServer.value.url,
      room.id,
      displayName.value || 'Me',
      selectedCapture.value,
      selectedPlayback.value,
    )
    activeRoomID.value   = room.id
    activeRoomName.value = room.name
    connected.value = true
    muted.value     = false
    loopbackEnabled.value = false
    saveConfig()
  } catch (e: any) {
    error.value = e?.message ?? String(e)
  }
}

async function leaveRoom() {
  try { await Session.disconnect() } catch { /* ignore */ }
  connected.value = false
  activeRoomID.value = ''
  activeRoomName.value = ''
  roomMembers.value = []
  loopbackEnabled.value = false
  noiseSuppressionEnabled.value = false
}

// ── Audio actions ─────────────────────────────────────────────────────────────

async function toggleMute() {
  muted.value = !muted.value
  await Session.setMuted(muted.value)
}

async function toggleLoopback() {
  const newVal = !loopbackEnabled.value
  settingsError.value = ''
  try {
    if (connected.value) {
      await Session.setLoopback(newVal)
    } else if (newVal) {
      await Audio.startLoopback(selectedCapture.value, selectedPlayback.value)
    } else {
      await Audio.stopLoopback()
    }
    loopbackEnabled.value = newVal
  } catch (e: any) {
    settingsError.value = e?.message ?? String(e)
  }
}

async function toggleNoiseSuppression() {
  settingsError.value = ''
  const newVal = !noiseSuppressionEnabled.value
  try {
    await Session.setNoiseSuppression(newVal)
    noiseSuppressionEnabled.value = await Session.isNoiseSuppressionEnabled() ?? false
    saveConfig()
  } catch (e: any) {
    settingsError.value = e?.message ?? String(e)
  }
}

async function onMicGainChange() {
  await Session.setMicGainDB(micGainDB.value)
  saveConfig()
}

async function onVAChange() {
  await Session.setVoiceActivationThresholdDB(voiceActivationDB.value)
  saveConfig()
}

async function loadDevices() {
  try {
    devices.value = (await Audio.listDevices()) ?? { capture: [], playback: [] }
  } catch { /* non-fatal */ }
}

// ── Settings helpers ──────────────────────────────────────────────────────────

function openSettings(tab: typeof settingsTab.value) {
  settingsTab.value = tab
  settingsOpen.value = true
}

// ── Config ────────────────────────────────────────────────────────────────────

async function loadConfig() {
  try {
    const cfg = await Config.load()
    if (cfg.display_name)              displayName.value       = cfg.display_name
    if (cfg.capture_device_name)       selectedCapture.value   = cfg.capture_device_name
    if (cfg.playback_device_name)      selectedPlayback.value  = cfg.playback_device_name
    if (cfg.mic_gain_db != null)       micGainDB.value         = cfg.mic_gain_db
    if (cfg.voice_activation_threshold_db != null) voiceActivationDB.value = cfg.voice_activation_threshold_db
    if (cfg.servers?.length)           servers.value           = cfg.servers
    if (cfg.last_used_server?.http_url) {
      const url = cfg.last_used_server.http_url
      activeServer.value = { name: cfg.last_used_server.name || url, url }
      await refreshRooms()
      await Events.subscribe(url)
    }
  } catch { /* missing config is fine */ }
}

async function saveConfig() {
  try {
    await Config.save({
      version: 1,
      display_name:              displayName.value,
      capture_device_name:       selectedCapture.value,
      playback_device_name:      selectedPlayback.value,
      mic_gain_db:               micGainDB.value,
      voice_activation_threshold_db: voiceActivationDB.value,
      last_used_server: activeServer.value
        ? { name: activeServer.value.name, http_url: activeServer.value.url }
        : undefined,
      servers: servers.value,
    })
  } catch { /* non-fatal */ }
}

// ── Utils ─────────────────────────────────────────────────────────────────────

function initials(name: string) {
  return name.split(/\s+/).map(w => w[0]).join('').slice(0, 2).toUpperCase() || '?'
}

// Close settings on Escape
function onKeydown(e: KeyboardEvent) { if (e.key === 'Escape') settingsOpen.value = false }
onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<style>
* { box-sizing: border-box; margin: 0; padding: 0; }

body {
  font-family: system-ui, -apple-system, sans-serif;
  background: #111;
  color: #dcddde;
  height: 100vh;
  overflow: hidden;
  user-select: none;
}

/* ── Layout ────────────────────────────────────────────────────────────────── */

.app {
  display: grid;
  grid-template-columns: 240px 1fr;
  grid-template-rows: 1fr 44px;
  height: 100vh;
}

/* ── Sidebar ────────────────────────────────────────────────────────────────── */

.sidebar {
  grid-row: 1 / 3;
  background: #1e1f22;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-right: 1px solid #2b2d31;
}

.server-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  border-bottom: 1px solid #2b2d31;
  font-weight: 700;
  font-size: 15px;
  flex-shrink: 0;
}

.server-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.section-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 8px 4px 14px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: .06em;
  color: #949ba4;
  flex-shrink: 0;
}

.new-room-row {
  display: flex;
  gap: 6px;
  padding: 4px 8px;
  flex-shrink: 0;
}
.new-room-row input {
  flex: 1;
  background: #111214;
  border: 1px solid #4e5058;
  border-radius: 4px;
  color: #dcddde;
  padding: 5px 8px;
  font-size: 13px;
}
.new-room-row button {
  background: #5865f2;
  border: none;
  border-radius: 4px;
  color: #fff;
  padding: 5px 10px;
  cursor: pointer;
  font-size: 12px;
}

.channel-list {
  list-style: none;
  overflow-y: auto;
  flex: 1;
  padding: 0 6px;
}

.channel-item {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 5px 8px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
  color: #949ba4;
  position: relative;
}
.channel-item:hover { background: #35373c; color: #dcddde; }
.channel-item.active { background: #404249; color: #f2f3f5; }
.hash { font-size: 15px; color: #4e5058; flex-shrink: 0; }
.ch-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ch-count {
  font-size: 11px;
  background: #4e5058;
  border-radius: 8px;
  padding: 1px 6px;
  min-width: 20px;
  text-align: center;
}
.ch-delete {
  display: none;
  background: none;
  border: none;
  color: #949ba4;
  cursor: pointer;
  font-size: 12px;
  padding: 2px 4px;
  border-radius: 3px;
  flex-shrink: 0;
}
.channel-item:hover .ch-delete { display: block; }
.ch-delete:hover { background: #da373c33; color: #f23f43; }

.sidebar-spacer { flex: 1; min-height: 0; }

.user-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  background: #232428;
  border-top: 1px solid #2b2d31;
  flex-shrink: 0;
}
.user-name {
  flex: 1;
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mute-btn {
  background: none;
  border: none;
  font-size: 18px;
  cursor: pointer;
  padding: 3px 5px;
  border-radius: 4px;
  opacity: 1;
  transition: opacity .15s;
}
.mute-btn:hover { background: #35373c; }
.mute-btn.muted { opacity: .45; }
.mute-btn:disabled { cursor: not-allowed; opacity: .25; }

.icon-btn {
  background: none;
  border: none;
  color: #949ba4;
  cursor: pointer;
  font-size: 16px;
  padding: 3px 5px;
  border-radius: 4px;
  flex-shrink: 0;
}
.icon-btn:hover { background: #35373c; color: #dcddde; }

/* ── Main ───────────────────────────────────────────────────────────────────── */

.main {
  background: #313338;
  overflow-y: auto;
}

.room-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 20px;
  border-bottom: 1px solid #2b2d31;
  font-weight: 700;
  font-size: 16px;
  position: sticky;
  top: 0;
  background: #313338;
  z-index: 1;
}
.leave-btn {
  margin-left: auto;
  background: #da373c;
  border: none;
  border-radius: 4px;
  color: #fff;
  padding: 5px 14px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
}
.leave-btn:hover { background: #a12828; }

.members-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  padding: 24px 20px;
}
.member-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  width: 80px;
}
.member-avatar {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: #5865f2;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  font-weight: 700;
  color: #fff;
}
.member-name {
  font-size: 12px;
  text-align: center;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  width: 100%;
}
.empty-hint { color: #4e5058; font-size: 14px; padding: 8px 0; }

.welcome {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 8px;
}
.welcome-icon { font-size: 48px; }
.welcome-title { font-size: 20px; font-weight: 700; }
.welcome-sub { color: #949ba4; font-size: 14px; }

/* ── Audio bar ──────────────────────────────────────────────────────────────── */

.audio-bar {
  grid-column: 2;
  background: #232428;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 16px;
  border-top: 1px solid #2b2d31;
}

.level-track {
  flex: 1;
  height: 4px;
  background: #111214;
  border-radius: 2px;
  overflow: hidden;
}
.level-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 80ms linear;
}
.level-low  { background: #23a55a; }
.level-ok   { background: #f0b232; }
.level-hot  { background: #da373c; }

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.status-dot.online  { background: #23a55a; }
.status-dot.offline { background: #80848e; }

.status-text { font-size: 12px; color: #949ba4; white-space: nowrap; }

.error-pill {
  background: #5c1b1b;
  color: #f28b82;
  border-radius: 12px;
  padding: 2px 10px;
  font-size: 11px;
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ── Settings overlay ───────────────────────────────────────────────────────── */

.overlay {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}

.settings-modal {
  display: flex;
  width: min(900px, 95vw);
  height: min(660px, 90vh);
  background: #313338;
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 0 8px 40px rgba(0,0,0,.6);
}

.settings-nav {
  width: 200px;
  flex-shrink: 0;
  background: #2b2d31;
  display: flex;
  flex-direction: column;
  padding: 16px 8px 8px;
  gap: 2px;
}
.settings-nav-group { margin-bottom: 16px; display: flex; flex-direction: column; gap: 2px; }
.settings-nav-label {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: .06em;
  color: #949ba4;
  padding: 4px 8px;
}
.settings-nav button {
  background: none;
  border: none;
  color: #949ba4;
  cursor: pointer;
  text-align: left;
  padding: 7px 10px;
  border-radius: 4px;
  font-size: 14px;
}
.settings-nav button:hover { background: #35373c; color: #dcddde; }
.settings-nav button.active { background: #404249; color: #f2f3f5; }
.settings-close {
  margin-top: auto !important;
  color: #949ba4 !important;
  font-size: 12px !important;
}

.settings-body {
  flex: 1;
  overflow-y: auto;
  padding: 24px 32px;
}

.settings-section h2 {
  font-size: 20px;
  font-weight: 700;
  margin-bottom: 24px;
  color: #f2f3f5;
}
.settings-section h3 {
  font-size: 14px;
  font-weight: 700;
  margin: 20px 0 10px;
  color: #f2f3f5;
}

.field {
  margin-bottom: 20px;
}
.field label {
  display: block;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: .04em;
  color: #b5bac1;
  margin-bottom: 8px;
}
.field input[type=text], .field input:not([type=range]):not([type=checkbox]),
.field select {
  width: 100%;
  background: #1e1f22;
  border: 1px solid #1e1f22;
  border-radius: 4px;
  color: #dcddde;
  padding: 10px 12px;
  font-size: 14px;
  outline: none;
}
.field input:focus, .field select:focus {
  border-color: #5865f2;
}
.field input[type=range] {
  width: 100%;
  cursor: pointer;
  accent-color: #5865f2;
}
.field-hint {
  font-size: 12px;
  color: #949ba4;
  margin-top: 6px;
}
.field-hint.inline { display: inline; margin: 0 0 0 8px; font-weight: 400; text-transform: none; letter-spacing: 0; }
.value-badge {
  display: inline-block;
  background: #404249;
  border-radius: 4px;
  padding: 1px 6px;
  font-size: 11px;
  font-weight: 400;
  text-transform: none;
  letter-spacing: 0;
  margin-left: 6px;
  vertical-align: middle;
}

.toggle-field { display: flex; align-items: flex-start; gap: 16px; }
.toggle-field label { min-width: 140px; padding-top: 4px; }
.toggle-wrap { display: flex; flex-direction: column; gap: 4px; }
.toggle {
  background: #4e5058;
  border: none;
  border-radius: 4px;
  color: #dcddde;
  padding: 6px 16px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  min-width: 60px;
  transition: background .15s;
}
.toggle.on { background: #23a55a; color: #fff; }
.toggle:disabled { opacity: .4; cursor: not-allowed; }

.preview-level { height: 8px; border-radius: 4px; }

/* Servers */
.server-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 20px;
}
.server-row {
  display: flex;
  align-items: center;
  gap: 10px;
  background: #2b2d31;
  border-radius: 6px;
  padding: 10px 14px;
  border: 1px solid transparent;
}
.server-row.active { border-color: #5865f2; }
.srv-info { flex: 1; min-width: 0; }
.srv-name { font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; }
.srv-url  { font-size: 11px; color: #949ba4; overflow: hidden; text-overflow: ellipsis; }
.small-btn {
  background: #404249;
  border: none;
  border-radius: 4px;
  color: #dcddde;
  padding: 5px 12px;
  cursor: pointer;
  font-size: 12px;
  white-space: nowrap;
  flex-shrink: 0;
}
.small-btn:hover { background: #4e5058; }
.small-btn.danger:hover { background: #da373c; color: #fff; }

.add-server-form { border-top: 1px solid #2b2d31; padding-top: 16px; }
.primary-btn {
  background: #5865f2;
  border: none;
  border-radius: 4px;
  color: #fff;
  padding: 10px 20px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 600;
}
.primary-btn:hover { background: #4752c4; }

/* Fade transition */
.fade-enter-active, .fade-leave-active { transition: opacity .15s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
