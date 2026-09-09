// API Types matching Gateway

export interface User {
  id: string;
  email: string;
  created_at: string;
}

export interface AuthResponse {
  token: string;
  expires_in: number;
  user: User;
}

export interface Site {
	hosting_status?: 'provisioning' | 'ready';
	live_url: string;
	preview_url: string;
	initialization_status: 'legacy' | 'pending' | 'ready';
  id: string;
  fqdn: string;
  user_id: string;
  template_id: string;
  live_version_id?: string;
  preview_version_id?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface SiteAlias {
  id: string;
  site_id: string;
  alias: string;
  created_at: string;
}

export interface Version {
  id: string;
  site_id: string;
  build_id: string;
  status: 'completed';
  created_at: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  page: number;
  page_size: number;
  total_count: number;
  total_pages: number;
}

export type JobStatus = 'pending' | 'running' | 'completed' | 'failed';

export interface BuildHistoryItem {
  job_id: string;
  site_id: string;
  source_version: string;
  target_version: string;
  status: JobStatus;
  dispatch_state: 'ready' | 'dispatching' | 'accepted' | 'rejected';
  error_code?: string;
  recovery_error?: string;
  created_at: string;
  updated_at: string;
}

export interface AcceptedBuildResponse {
  job_id: string;
  site_id: string;
  owner_id: string;
  source_version: string;
  target_version: string;
  status: JobStatus;
  error_message?: string;
}

export interface JobSnapshot extends AcceptedBuildResponse {
  prompt: string;
  created_at: string;
  updated_at: string;
  result?: string;
  error_message?: string;
  manifest_path?: string;
}

// Request Types

export interface RegisterRequest {
  email: string;
  password: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface ForgotPasswordRequest {
  email: string;
}

export interface ResetPasswordRequest {
  token: string;
  password: string;
}

export interface UpdatePasswordRequest {
  current_password: string;
  new_password: string;
}

export interface CreateSiteRequest {
  fqdn: string;
  template_id: string;
}

export interface AddAliasRequest {
  alias: string;
}

export interface DeployVersionRequest {
  target: 'live' | 'preview';
}

export interface BuildRequest {
  message: string;
  conversation_id?: string;
}

export interface ClarificationBuildResponse {
  question: string;
  conversation_id: string;
}

export type BuildResponse = AcceptedBuildResponse | ClarificationBuildResponse;

export interface ErrorResponse {
  error: string;
  message?: string;
}
