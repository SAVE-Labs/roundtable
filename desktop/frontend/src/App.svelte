<script>
  import { onMount } from 'svelte'
  import {
    ListAudioDevices,
    ListRooms,
    CreateRoom,
    JoinRoom,
    LeaveRoom,
    SessionStatus,
  } from '../wailsjs/go/main/App.js'

  let serverURL = 'http://127.0.0.1:1323'
  let wsURL = ''

  let rooms = []
  let roomsError = ''

  let captureDevices = []
  let playbackDevices = []
  let devicesError = ''
  let captureIndex = -1
  let playbackIndex = -1

  let newRoomName = ''
  let status = 'Not connected'
  let connected = false
  let joinError = ''

  function deriveWsURL(base) {
    return base.replace(/^http/, 'ws').replace(/\/?$/, '/ws')
  }

  $: wsURL = deriveWsURL(serverURL)

  async function loadRooms() {
    roomsError = ''
    try {
      rooms = await ListRooms(serverURL)
      if (!rooms) rooms = []
    } catch (e) {
      roomsError = String(e)
      rooms = []
    }
  }

  async function createRoom() {
    if (!newRoomName.trim()) return
    try {
      const room = await CreateRoom(serverURL, newRoomName.trim())
      newRoomName = ''
      await loadRooms()
      return room
    } catch (e) {
      roomsError = String(e)
    }
  }

  async function loadDevices() {
    devicesError = ''
    try {
      const result = await ListAudioDevices()
      captureDevices = result.capture || []
      playbackDevices = result.playback || []
      captureIndex = captureDevices.length > 0 ? 0 : -1
      playbackIndex = playbackDevices.length > 0 ? 0 : -1
    } catch (e) {
      devicesError = String(e)
    }
  }

  async function joinRoom(roomID) {
    joinError = ''
    if (captureIndex < 0 || playbackIndex < 0) {
      joinError = 'Select capture and playback devices first'
      return
    }
    const url = wsURL + '?room=' + roomID
    try {
      await JoinRoom(url, captureIndex, playbackIndex)
      connected = true
      status = await SessionStatus()
    } catch (e) {
      joinError = String(e)
    }
  }

  async function leaveRoom() {
    await LeaveRoom()
    connected = false
    status = await SessionStatus()
  }

  onMount(async () => {
    await loadDevices()
    await loadRooms()
  })
</script>

<main>
  <header>
    <h1>Roundtable</h1>
    <div class="status-bar" class:connected>
      {status}
      {#if connected}
        <button class="btn-leave" on:click={leaveRoom}>Leave</button>
      {/if}
    </div>
  </header>

  <div class="layout">
    <!-- Left panel: server + rooms -->
    <section class="panel">
      <div class="section-header">
        <h2>Server</h2>
      </div>
      <div class="field-row">
        <input
          type="text"
          bind:value={serverURL}
          placeholder="http://127.0.0.1:1323"
          on:change={loadRooms}
        />
      </div>

      <div class="section-header">
        <h2>Rooms</h2>
        <button class="btn-icon" on:click={loadRooms} title="Refresh">⟳</button>
      </div>

      {#if roomsError}
        <p class="error">{roomsError}</p>
      {/if}

      <div class="room-list">
        {#each rooms as room (room.id)}
          <div class="room-row">
            <span class="room-name">{room.name}</span>
            <button
              class="btn-join"
              on:click={() => joinRoom(room.id)}
              disabled={connected}
            >Join</button>
          </div>
        {:else}
          <p class="muted">No rooms. Create one below.</p>
        {/each}
      </div>

      <div class="create-room">
        <input
          type="text"
          bind:value={newRoomName}
          placeholder="New room name"
          on:keydown={(e) => e.key === 'Enter' && createRoom()}
        />
        <button class="btn-create" on:click={createRoom}>Create</button>
      </div>
    </section>

    <!-- Right panel: audio devices -->
    <section class="panel">
      <div class="section-header">
        <h2>Audio Devices</h2>
        <button class="btn-icon" on:click={loadDevices} title="Refresh">⟳</button>
      </div>

      {#if devicesError}
        <p class="error">{devicesError}</p>
      {/if}

      <label>
        Microphone (capture)
        <select bind:value={captureIndex} disabled={connected}>
          {#each captureDevices as dev, i}
            <option value={i}>{dev.name}</option>
          {:else}
            <option disabled>No capture devices</option>
          {/each}
        </select>
      </label>

      <label>
        Speaker (playback)
        <select bind:value={playbackIndex} disabled={connected}>
          {#each playbackDevices as dev, i}
            <option value={i}>{dev.name}</option>
          {:else}
            <option disabled>No playback devices</option>
          {/each}
        </select>
      </label>

      {#if joinError}
        <p class="error">{joinError}</p>
      {/if}

      <p class="hint">Select devices, then click Join on a room.</p>
    </section>
  </div>
</main>

<style>
  :global(body) {
    margin: 0;
    font-family: system-ui, -apple-system, sans-serif;
    background: #1a1a2e;
    color: #e0e0e0;
  }

  main {
    display: flex;
    flex-direction: column;
    height: 100vh;
  }

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 20px;
    background: #16213e;
    border-bottom: 1px solid #0f3460;
  }

  h1 {
    margin: 0;
    font-size: 1.3rem;
    color: #e94560;
  }

  h2 {
    margin: 0;
    font-size: 0.95rem;
    color: #a0a0c0;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .status-bar {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 0.85rem;
    color: #888;
  }

  .status-bar.connected {
    color: #4caf50;
  }

  .layout {
    display: flex;
    flex: 1;
    gap: 0;
    overflow: hidden;
  }

  .panel {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 16px;
    overflow-y: auto;
    border-right: 1px solid #0f3460;
  }

  .panel:last-child {
    border-right: none;
  }

  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  input[type='text'] {
    width: 100%;
    box-sizing: border-box;
    padding: 7px 10px;
    background: #0f3460;
    border: 1px solid #1a4a7a;
    border-radius: 4px;
    color: #e0e0e0;
    font-size: 0.9rem;
  }

  input[type='text']:focus {
    outline: none;
    border-color: #e94560;
  }

  select {
    width: 100%;
    box-sizing: border-box;
    padding: 7px 10px;
    background: #0f3460;
    border: 1px solid #1a4a7a;
    border-radius: 4px;
    color: #e0e0e0;
    font-size: 0.9rem;
    margin-top: 4px;
  }

  select:disabled {
    opacity: 0.5;
  }

  label {
    display: flex;
    flex-direction: column;
    font-size: 0.85rem;
    color: #a0a0c0;
  }

  .field-row {
    display: flex;
    gap: 8px;
  }

  .room-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-height: 60px;
  }

  .room-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 10px;
    background: #0f3460;
    border-radius: 4px;
  }

  .room-name {
    font-size: 0.95rem;
  }

  .create-room {
    display: flex;
    gap: 8px;
  }

  .create-room input {
    flex: 1;
  }

  .btn-join {
    padding: 4px 12px;
    background: #e94560;
    color: white;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.85rem;
  }

  .btn-join:disabled {
    opacity: 0.4;
    cursor: default;
  }

  .btn-join:hover:not(:disabled) {
    background: #c73652;
  }

  .btn-create {
    padding: 7px 14px;
    background: #0f3460;
    color: #e0e0e0;
    border: 1px solid #1a4a7a;
    border-radius: 4px;
    cursor: pointer;
    white-space: nowrap;
  }

  .btn-create:hover {
    background: #1a4a7a;
  }

  .btn-leave {
    padding: 3px 10px;
    background: transparent;
    color: #4caf50;
    border: 1px solid #4caf50;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.8rem;
  }

  .btn-leave:hover {
    background: #4caf5022;
  }

  .btn-icon {
    background: none;
    border: none;
    color: #a0a0c0;
    cursor: pointer;
    font-size: 1.1rem;
    padding: 0 4px;
  }

  .btn-icon:hover {
    color: #e0e0e0;
  }

  .error {
    color: #e94560;
    font-size: 0.85rem;
    margin: 0;
  }

  .muted {
    color: #666;
    font-size: 0.85rem;
    margin: 0;
  }

  .hint {
    color: #666;
    font-size: 0.8rem;
    margin: 0;
  }
</style>
