// Typed wrappers around Wails v3 Go service bindings.
// FQN format: "packagePath.TypeName.MethodName"
// packagePath = github.com/SAVE-Labs/roundtable/desktop/services

import { Call } from "@wailsio/runtime"

const pkg = "github.com/SAVE-Labs/roundtable/desktop/services"

function call<T>(service: string, method: string, ...args: unknown[]): Promise<T> {
  return Call.ByName(`${pkg}.${service}.${method}`, ...args) as Promise<T>
}

// ── Shared types ─────────────────────────────────────────────────────────────

export interface Room {
  id: string
  name: string
  member_count: number
}

export interface ServerInfo {
  name: string
  version: string
}

export interface DeviceInfo {
  name: string
  is_default: boolean
}

export interface AudioDevices {
  capture: DeviceInfo[]
  playback: DeviceInfo[]
}

export interface ServerConfig {
  name?: string
  http_url?: string
  ws_url?: string
}

export interface AppConfig {
  version?: number
  display_name?: string
  capture_device_name?: string
  playback_device_name?: string
  mic_muted?: boolean
  voice_activation_threshold_db?: number
  mic_gain_db?: number
  last_used_server?: ServerConfig
  servers?: ServerConfig[]
}

// ── RoomsService ─────────────────────────────────────────────────────────────

export const Rooms = {
  list:   (serverURL: string)               => call<Room[]>('RoomsService', 'ListRooms', serverURL),
  create: (serverURL: string, name: string) => call<Room>('RoomsService', 'CreateRoom', serverURL, name),
  delete: (serverURL: string, id: string)   => call<void>('RoomsService', 'DeleteRoom', serverURL, id),
  probe:  (serverURL: string)               => call<ServerInfo>('RoomsService', 'ProbeServer', serverURL),
}

// ── AudioService ─────────────────────────────────────────────────────────────

export const Audio = {
  listDevices:      ()                                         => call<AudioDevices>('AudioService', 'ListDevices'),
  startLoopback:    (capture: string, playback: string)        => call<void>('AudioService', 'StartLoopback', capture, playback),
  stopLoopback:     ()                                         => call<void>('AudioService', 'StopLoopback'),
  isLoopbackActive: ()                                         => call<boolean>('AudioService', 'IsLoopbackActive'),
}

// ── SessionService ───────────────────────────────────────────────────────────

export const Session = {
  connect:     (serverURL: string, roomID: string, peerName: string, capture: string, playback: string) =>
                 call<void>('SessionService', 'Connect', serverURL, roomID, peerName, capture, playback),
  disconnect:  ()              => call<void>('SessionService', 'Disconnect'),
  isConnected: ()              => call<boolean>('SessionService', 'IsConnected'),

  setMuted:    (v: boolean)    => call<void>('SessionService', 'SetMuted', v),
  isMuted:     ()              => call<boolean>('SessionService', 'IsMuted'),

  setLoopback: (v: boolean)    => call<void>('SessionService', 'SetLoopback', v),
  isLoopback:  ()              => call<boolean>('SessionService', 'IsLoopback'),

  setNoiseSuppression:        (v: boolean) => call<void>('SessionService', 'SetNoiseSuppression', v),
  isNoiseSuppressionEnabled:  ()           => call<boolean>('SessionService', 'IsNoiseSuppressionEnabled'),
  isNoiseSuppressionAvailable: ()          => call<boolean>('SessionService', 'IsNoiseSuppressionAvailable'),

  setMicGainDB:                  (db: number) => call<void>('SessionService', 'SetMicGainDB', db),
  setVoiceActivationThresholdDB: (db: number) => call<void>('SessionService', 'SetVoiceActivationThresholdDB', db),
  getMicLevelDB:                 ()           => call<number>('SessionService', 'GetMicLevelDB'),
}

// ── ConfigService ─────────────────────────────────────────────────────────────

export const Config = {
  load: ()               => call<AppConfig>('ConfigService', 'Load'),
  save: (cfg: AppConfig) => call<void>('ConfigService', 'Save', cfg),
}

// ── EventsService ─────────────────────────────────────────────────────────────

export const Events = {
  subscribe:   (serverURL: string) => call<void>('EventsService', 'Subscribe', serverURL),
  unsubscribe: ()                  => call<void>('EventsService', 'Unsubscribe'),
}
