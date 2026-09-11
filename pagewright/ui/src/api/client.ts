import axios, { type AxiosInstance, AxiosError } from 'axios';
import { config } from '../config';
import { parseSiteDomain } from './capabilities';
import { isSessionExpiry, rememberReturn, readOwner } from './drafts';
import { parseSiteHosting, parseBuildResponse, parseVersionPage, parseBuildHistory, parseBuildHistoryItem, parseDeployment } from './contracts';
import type {
  AuthResponse,
  RegisterRequest,
  LoginRequest,
  ForgotPasswordRequest,
  ResetPasswordRequest,
  UpdatePasswordRequest,
  Site,
  CreateSiteRequest,
  PaginatedResponse,
  Version,
  DeployVersionRequest,
  BuildRequest,
  BuildResponse,
  ErrorResponse,
} from '../types/api';

class ApiClient {
  private client: AxiosInstance;

  constructor() {
    this.client = axios.create({
      baseURL: config.apiUrl,
      timeout: 30000,
      headers: {
        'Content-Type': 'application/json',
      },
    });

    // Add request interceptor to include auth token
    this.client.interceptors.request.use((config) => {
      const draftOwner = config.headers.get('X-Pagewright-Draft-Owner');
      config.headers.delete('X-Pagewright-Draft-Owner');
      if (draftOwner && readOwner(localStorage) !== draftOwner) {
        throw new Error('The signed-in account changed. Reload before sending this draft.');
      }
      const token = localStorage.getItem('token');
      if (token) {
        config.headers.Authorization = `Bearer ${token}`;
      }
      return config;
    });

    // Add response interceptor for error handling
    this.client.interceptors.response.use(
      (response) => response,
      (error: AxiosError<ErrorResponse>) => {
        if (isSessionExpiry(error.response?.status, error.config?.url,
          error.config?.headers.Authorization, localStorage.getItem('token'))) {
          try {
            const user = JSON.parse(localStorage.getItem('user') || 'null');
            if (typeof user?.id === 'string') rememberReturn(sessionStorage, user.id, window.location.pathname);
          } catch { /* Saved drafts remain intact even if the return path cannot be saved. */ }
          localStorage.removeItem('token');
          localStorage.removeItem('user');
          window.location.replace('/login');
        }
        return Promise.reject(error);
      }
    );
  }

  // Auth endpoints
  async register(data: RegisterRequest): Promise<AuthResponse> {
    const response = await this.client.post<AuthResponse>('/auth/register', data);
    return response.data;
  }

  async login(data: LoginRequest): Promise<AuthResponse> {
    const response = await this.client.post<AuthResponse>('/auth/login', data);
    return response.data;
  }

  async forgotPassword(data: ForgotPasswordRequest): Promise<{ message: string }> {
    const response = await this.client.post('/auth/forgot-password', data);
    return response.data;
  }

  async resetPassword(data: ResetPasswordRequest): Promise<{ message: string }> {
    const response = await this.client.post('/auth/reset-password', data);
    return response.data;
  }

  async updatePassword(data: UpdatePasswordRequest): Promise<{ message: string }> {
    const response = await this.client.post('/auth/update-password', data);
    return response.data;
  }

  // Sites endpoints
  async getSiteDomain(signal?: AbortSignal): Promise<string> {
    const response = await this.client.get<unknown>('/capabilities', { signal, timeout: 10000 });
    return parseSiteDomain(response.data);
  }

  async createSite(data: CreateSiteRequest): Promise<Site> {
    const response = await this.client.post<Site>('/sites', data);
    return parseSiteHosting(response.data);
  }

  async listSites(page = 1, pageSize = 25, signal?: AbortSignal): Promise<PaginatedResponse<Site>> {
    const response = await this.client.get<PaginatedResponse<Site>>('/sites', {
      params: { page, page_size: pageSize },
      signal, timeout: 10000,
    });
    return { ...response.data, data: response.data.data.map(site => parseSiteHosting(site)) };
  }

  async getSite(fqdn: string, signal?: AbortSignal): Promise<Site> {
    const response = await this.client.get<Site>(`/sites/${encodeURIComponent(fqdn)}`, { signal, timeout: 10000 });
    return parseSiteHosting(response.data, fqdn);
  }

  async enableSite(fqdn: string): Promise<void> {
    await this.client.post(`/sites/${encodeURIComponent(fqdn)}/enable`, undefined, { timeout: 30000 });
  }

  async disableSite(fqdn: string): Promise<void> {
    await this.client.post(`/sites/${encodeURIComponent(fqdn)}/disable`, undefined, { timeout: 30000 });
  }

  // Versions endpoints
  async listVersions(fqdn: string, page = 1, pageSize = 25, signal?: AbortSignal): Promise<PaginatedResponse<Version>> {
    const response = await this.client.get<PaginatedResponse<Version>>(`/sites/${fqdn}/versions`, {
      params: { page, page_size: pageSize },
      signal, timeout: 10000,
    });
    return parseVersionPage(response.data);
  }

  async deployVersion(fqdn: string, versionId: string, data: DeployVersionRequest) {
    const response = await this.client.post<unknown>(`/sites/${encodeURIComponent(fqdn)}/versions/${encodeURIComponent(versionId)}/deploy`, data, { timeout: 30000 });
    return parseDeployment(response.data, fqdn, versionId, data.target);
  }

  async downloadVersion(fqdn: string, versionId: string): Promise<Blob> {
    const response = await this.client.get(`/sites/${fqdn}/versions/${versionId}/download`, {
      responseType: 'blob',
    });
    return response.data;
  }

  // Build endpoint
  async listJobs(fqdn: string, page = 1, signal?: AbortSignal) {
    const response = await this.client.get<unknown>(`/sites/${encodeURIComponent(fqdn)}/jobs`, { params: { page, page_size: 25 }, signal, timeout: 10000 });
    const history = parseBuildHistory(response.data);
    if (history.page !== page || history.page_size !== 25) throw new Error('Unexpected history page');
    return history;
  }

  async getJob(fqdn: string, jobId: string, signal?: AbortSignal) {
    const response = await this.client.get<unknown>(`/sites/${encodeURIComponent(fqdn)}/jobs/${encodeURIComponent(jobId)}`, { signal, timeout: 10000 });
    const job = parseBuildHistoryItem(response.data);
    if (job.job_id !== jobId) throw new Error('Unexpected job identity');
    return job;
  }

  async registrationOpen(signal?: AbortSignal): Promise<boolean> {
    const response = await this.client.get<{ registration_open?: boolean }>('/capabilities', { signal, timeout: 10000 });
    return response.data.registration_open === true;
  }

  async build(fqdn: string, data: BuildRequest & { requestKey: string }, owner: string): Promise<BuildResponse> {
    const payload: BuildRequest = { message: data.message, conversation_id: data.conversation_id };
    const response = await this.client.post<unknown>(`/sites/${fqdn}/build`, payload, {
      headers: { 'Idempotency-Key': data.requestKey, 'X-Pagewright-Draft-Owner': owner },
      timeout: 60000,
    });
    return parseBuildResponse(response.data);
  }
}

export const apiClient = new ApiClient();
