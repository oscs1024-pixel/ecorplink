import type {
  AppState,
  CommandResult,
  ConfigDocument,
  ConfigResponse,
  DaemonStatus,
  LaunchServiceRequest,
  LogChunk,
  LoginMethodsResult,
  QRCodeResult,
  ServiceStatus,
  TestReport,
  ValidationResult,
  VPNNodesResult,
  VPNStatsResult,
  VPNStatusResult,
  WindowState
} from '../types'
import * as Service from '../bindings/ecorplink/internal/gui/service'

type Backend = Record<string, (...args: never[]) => Promise<unknown>>

async function call<T>(name: string, ...args: unknown[]): Promise<T> {
  const fn = (Service as unknown as Backend)[name]
  if (!fn) {
    throw new Error(`后端方法不存在：${name}`)
  }
  try {
    return (await fn(...(args as never[]))) as T
  } catch (error) {
    throw new Error(`后端调用失败：${name}: ${errorMessage(error)}`)
  }
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message
  }
  if (typeof error === 'string') {
    return error
  }
  try {
    return JSON.stringify(error)
  } catch {
    return String(error)
  }
}

export const api = {
  getAppState(): Promise<AppState> {
    return call('GetAppState')
  },
  loadConfig(path = ''): Promise<ConfigResponse> {
    return call('LoadConfig', path)
  },
  saveConfig(path: string, config: ConfigDocument): Promise<ValidationResult> {
    return call('SaveConfig', path, config)
  },
  validateConfig(config: ConfigDocument): Promise<ValidationResult> {
    return call('ValidateConfig', config)
  },
  reloadConfig(): Promise<CommandResult> {
    return call('ReloadConfig')
  },
  startDaemon(path = ''): Promise<CommandResult> {
    return call('StartDaemon', path)
  },
  stopDaemon(): Promise<CommandResult> {
    return call('StopDaemon')
  },
  restartDaemon(path = ''): Promise<CommandResult> {
    return call('RestartDaemon', path)
  },
  getDaemonStatus(): Promise<DaemonStatus> {
    return call('GetDaemonStatus')
  },
  runConnectivityTests(): Promise<TestReport> {
    return call('RunConnectivityTests', {})
  },
  readLog(lines = 200): Promise<LogChunk> {
    return call('ReadLog', { lines })
  },
  getLaunchServiceStatus(): Promise<ServiceStatus> {
    return call('GetLaunchServiceStatus')
  },
  installLaunchService(req: LaunchServiceRequest): Promise<CommandResult> {
    return call('InstallLaunchService', req)
  },
  uninstallLaunchService(): Promise<CommandResult> {
    return call('UninstallLaunchService')
  },
  getWindowState(): Promise<WindowState> {
    return call('GetWindowState')
  },
  saveWindowState(state: WindowState): Promise<CommandResult> {
    return call('SaveWindowState', state)
  },
  getVersion(): Promise<string> {
    return call('GetVersion')
  },

  // Corplink auth & VPN methods
  isAuthenticated(): Promise<boolean> {
    return call('IsAuthenticated')
  },
  discoverCompany(company: string): Promise<CommandResult> {
    return call('DiscoverCompany', company)
  },
  getLoginMethods(): Promise<LoginMethodsResult> {
    return call('GetLoginMethods')
  },
  sendVerifyCode(codeType: string, account: string): Promise<CommandResult> {
    return call('SendVerifyCode', codeType, account)
  },
  verifyCode(codeType: string, account: string, code: string): Promise<CommandResult> {
    return call('VerifyCode', codeType, account, code)
  },
  loginWithPassword(account: string, password: string): Promise<CommandResult> {
    return call('LoginWithPassword', account, password)
  },
  getQRCode(): Promise<QRCodeResult> {
    return call('GetQRCode')
  },
  pollQRStatus(token: string): Promise<CommandResult> {
    return call('PollQRStatus', token)
  },
  logout(): Promise<CommandResult> {
    return call('Logout')
  },
  listVPNNodes(): Promise<VPNNodesResult> {
    return call('ListVPNNodes')
  },
  pingNodes(): Promise<VPNNodesResult> {
    return call('PingNodes')
  },
  pingSingleNode(nodeID: number): Promise<VPNNodesResult> {
    return call('PingSingleNode', nodeID)
  },
  connectVPN(nodeID: number, followSplitRoutes: boolean): Promise<CommandResult> {
    return call('ConnectVPN', nodeID, followSplitRoutes)
  },
  setFollowSplitRoutes(v: boolean): Promise<CommandResult> {
    return call('SetFollowSplitRoutes', v)
  },
  disconnectVPN(): Promise<CommandResult> {
    return call('DisconnectVPN')
  },
  getVPNStatus(): Promise<VPNStatusResult> {
    return call('GetVPNStatus')
  },
  getVPNStats(): Promise<VPNStatsResult> {
    return call('GetVPNStats')
  },
  cleanupRoutes(): Promise<CommandResult> {
    return call('CleanupRoutes')
  },
  applyRecommendedRules(path = ''): Promise<CommandResult> {
    return call('ApplyRecommendedRules', path)
  }
}
