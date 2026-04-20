export type Id = string | number

export interface ApiResponse<T = any> {
  code: number
  data: T
  error?: string
  message?: string
}

export interface User {
  id: number
  name: string
  email: string
  role: 'admin' | 'member'
  telegram_id?: string
  department_id?: number
  department_name?: string
  group_id?: number
  group_name?: string
  employee_id?: string
  permissions?: string
  register_status?: string
  created_at: string
  last_login?: string
}

export interface TeamMember {
  id: number
  name: string
  department_name?: string
  group_name?: string
}

export interface Department {
  id: number
  name: string
  description?: string
}

export interface Group {
  id: number
  name: string
  department_id: number
  department_name?: string
}

export interface Site {
  id: number
  domain: string
  name: string
  gsc_site_url?: string
  ga_property_id?: string
  yandex_host_id?: string
  bing_site_url?: string
  status: string
  created_by?: number
  owner_name?: string
  last_audit_status?: string
  created_at: string
}

export interface AuditTask {
  id: number
  site_id: number
  status: string
  progress: number
  error_msg?: string
  created_at: string
  finished_at?: string
}

export type CoordStatus = 'pending' | 'pending_review' | 'in_progress' | 'awaiting_answer' | 'under_review' | 'done'

export interface CoordTask {
  id: number
  site_id: number
  site_name?: string
  issue_title: string
  issue_snapshot?: string
  issue_scope?: string
  status: CoordStatus
  priority?: string
  created_by: number
  creator_name?: string
  assignee_id?: number
  assignee_name?: string
  note?: string
  created_at: string
}

export interface CoordEvent {
  id: number
  task_id: number
  event_type: string
  from_status: string
  to_status: string
  actor_id: number
  actor_name: string
  note?: string
  created_at: string
}

export interface CoordAttachment {
  id: number
  task_id: number
  original_name: string
  content_type: string
  size_display?: string
  created_at: string
}

export interface DailyReport {
  id: number
  user_id: number
  report_date: string
  content: string
  polished_content?: string
  created_at: string
}

export interface WeeklyReport {
  id: number
  user_id: number
  title: string
  content: string
  status: string
  week_start: string
  week_end: string
  created_at: string
}

export interface MonthlyReport {
  id: number
  user_id: number
  title: string
  content: string
  status: string
  month: string
  created_at: string
}

export interface PermissionNode {
  code: string
  label: string
  children?: PermissionNode[]
  admin_only?: boolean
}

export interface OnlineUser {
  user_id: number
  email: string
  role: string
  ip: string
  last_seen: string
  session_count: number
}

export interface Settings {
  [key: string]: string
}

export interface PlatformHide {
  id: number
  platform: string
  site_key: string
}

export interface SiteIncident {
  id: number
  site_id: number
  incident_type: string
  title: string
  description?: string
  occurred_at: string
  created_at: string
}
