import { APIService } from '../bindings/exam_notify/desktop'

// 前端数据访问层：封装 Wails Binding 调用，统一 JSON 请求与错误抛出。

// 科目：学习主题。color 为预设主题色，sortOrder 控制排序，archived/deleted 标记软删除状态。
export type Subject = {
  id: number
  name: string
  color: string
  sortOrder: number | null
  archived: boolean
  deleted: boolean
  hasData?: boolean
}

// 可重置的数据表：描述存储模块名称、展示标签与记录数。
export type ResetTable = {
  name: string
  label: string
  count: number
}

// 学习计划：subjectId 关联科目，weekdays 为逗号分隔的星期（1-7），active 控制是否参与当天排程。
export type Plan = {
  id: number
  subjectId: number
  subject: string
  color: string
  startTime: string
  duration: number
  breakMinutes: number
  weekdays: string
  active: boolean
  note: string
}

// 学习会话：记录一次开始/暂停/结束的计时过程及其归属与来源。
export type Session = {
  id: number
  planId: number | null
  subjectId: number
  subject: string
  color: string
  startAt: string
  endAt: string | null
  plannedEndAt: string | null
  pausedAt: string | null
  breakStartedAt: string | null
  breakUsedSeconds: number
  resumedAt: string | null
  duration: number
  source: string
  note: string
}

// 活动记录：发生在某一天的一条文本记录。
export type Activity = {
  id: number
  happenedAt: string
  content: string
}

// 日志文件：名称、字节大小与最后修改时间。
export type ErrorLogFile = {
  name: string
  size: number
  modified: string
}

// 每日复盘中的单条科目项：计划/实际时长（秒）与补填原因。
export type ReviewItem = {
  subjectId: number
  subjectName: string
  plannedSeconds: number
  actualSeconds: number
  reason: string
}

// 每日复盘：按天汇总的计划/实际学习情况与逐项原因。
export type DailyReview = {
  day: string
  mode: 'total' | 'project'
  threshold: number
  plannedSeconds: number
  actualSeconds: number
  submittedAt?: string
  items: ReviewItem[]
}

// 批量复盘提示：待补原因的天数范围与数量。
export type BulkReviewOffer = {
  startDay: string
  endDay: string
  daysWithoutStudy: number
  pendingCount: number
}

// 后端把错误包装在返回 JSON 的 error 字段中，这里统一解析并抛出。
export async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const raw = await APIService.Call(
    path,
    method,
    body === undefined ? '' : JSON.stringify(body),
  )
  const val = JSON.parse(raw || 'null')
  if (val?.error) throw new Error(val.error)
  return val as T
}

// —— 每日复盘 ——
export const pendingDailyReview = () =>
  request<{ pending: DailyReview | null; bulk: BulkReviewOffer | null }>('daily-review/pending')

export const dailyReviewHistory = () => request<DailyReview[]>('daily-review/history')

export const submitDailyReview = (day: string, reasons: ReviewItem[]) =>
  request<{ ok: boolean }>('daily-review/submit', 'POST', { day, reasons })

export const submitBulkDailyReview = (startDay: string, endDay: string, reason: string) =>
  request<{ ok: boolean; count: number }>('daily-review/bulk', 'POST', { startDay, endDay, reason })

// —— 日志 ——
export const errorLogs = () =>
  request<{ directory: string; retentionDays: number; files: ErrorLogFile[] }>('logs')

export const readErrorLog = (name: string) =>
  request<{ content: string }>('logs?name=' + encodeURIComponent(name))

export const recordClientError = (message: string) =>
  request<{ ok: boolean }>('logs', 'POST', { message })

// —— 提醒与重置 ——
export const acknowledgeReminder = () => request<{ ok: boolean }>('reminder/ack', 'POST', {})

export const resetTables = () => request<ResetTable[]>('reset-tables')

export const resetTable = (name: string) =>
  request<{ ok: boolean }>('reset-tables?name=' + encodeURIComponent(name), 'DELETE')

// —— 聚合 API：Wails Binding 直连 + JSON 路由 ——
export const api = {
  homeReady: () => request<{ ok: boolean }>("startup/home-ready", "POST"),
  getAutostartEnabled: () => APIService.GetAutostartEnabled(),
  setAutostartEnabled: (enabled: boolean) => APIService.SetAutostartEnabled(enabled),
  getDataDirectory: () => APIService.GetDataDirectory(),
  chooseDataDirectory: () => APIService.ChooseDataDirectory(),
  saveHtml: (filename: string, content: string) => APIService.SaveHTML(filename, content),
  setDataDirectory: (dir: string) => APIService.SetDataDirectory(dir),

  subjects: (includeArchived = false, includeDeleted = false) => {
    const q = new URLSearchParams()
    if (includeArchived) q.set('includeArchived', 'true')
    if (includeDeleted) q.set('includeDeleted', 'true')
    return request<Subject[]>('subjects' + (q.size ? '?' + q.toString() : ''))
  },
  addSubject: (name: string, color: string, sortOrder: number | null) =>
    request<Subject>('subjects', 'POST', { name, color, sortOrder }),
  updateSubject: (subject: Subject) => request<Subject>('subjects', 'PUT', subject),
  deleteSubject: (id: number) => request('subjects?id=' + id, 'DELETE'),

  plans: () => request<Plan[]>('plans'),
  addPlan: (p: Partial<Plan>) => request<Plan>('plans', 'POST', p),
  updatePlan: (p: Plan) => request<Plan>('plans', 'PUT', p),
  deletePlan: (id: number) => request('plans?id=' + id, 'DELETE'),

  sessions: (date: string) => request<Session[]>('sessions?date=' + date),
  // 批量返回按本地自然日分组的记录，无记录日期仍保留空数组。
  sessionsForDates: (dates: string[]) => {
    const query = new URLSearchParams(dates.map(day => ['date', day]))
    return request<Record<string, Session[]>>('sessions/batch?' + query.toString())
  },
  start: (subjectId: number, planId: number) =>
    request<{ id: number }>('sessions/0', 'POST', { action: 'start', subjectId, planId }),
  finish: (id: number, action = 'stop') =>
    request<{ ok: boolean; saved: boolean }>('sessions/' + id, 'POST', { action }),
  deleteSession: (id: number) => request('sessions/' + id, 'DELETE'),
  addSession: (data: unknown) => request('sessions', 'POST', data),

  activities: (date: string) => request<Activity[]>('activities?date=' + date),
  addActivity: (content: string) =>
    request('activities', 'POST', { content }),

  settings: () => request<Record<string, string>>('settings'),
  saveSettings: (s: Record<string, string>) => request('settings', 'PUT', s),
}
