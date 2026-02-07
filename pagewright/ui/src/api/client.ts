import axios, { AxiosInstance, AxiosError } from 'axios';
import { config } from '../config';
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
  SiteAlias,
  AddAliasRequest,
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
      headers: {
        'Content-Type': 'application/json',
      },
    });

    // Add request interceptor to include auth token
    this.client.interceptors.request.use((config) => {
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
        if (error.response?.status === 401) {
          localStorage.removeItem('token');
          localStorage.removeItem('user');
          window.location.href = '/login';
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
  async createSite(data: CreateSiteRequest): Promise<Site> {
    const response = await this.client.post<Site>('/sites', data);
    return response.data;
  }

  async listSites(page = 1, pageSize = 25): Promise<PaginatedResponse<Site>> {
    const response = await this.client.get<PaginatedResponse<Site>>('/sites', {
      params: { page, page_size: pageSize },
    });
    return response.data;
  }

  async getSite(fqdn: string): Promise<Site> {
    const response = await this.client.get<Site>(`/sites/${fqdn}`);
    return response.data;
  }

  async deleteSite(fqdn: string): Promise<void> {
    await this.client.delete(`/sites/${fqdn}`);
  }

  async enableSite(fqdn: string): Promise<void> {
    await this.client.post(`/sites/${fqdn}/enable`);
  }

  async disableSite(fqdn: string): Promise<void> {
    await this.client.post(`/sites/${fqdn}/disable`);
  }

  // Aliases endpoints
  async listAliases(fqdn: string): Promise<SiteAlias[]> {
    const response = await this.client.get<SiteAlias[]>(`/sites/${fqdn}/aliases`);
    return response.data;
  }

  async addAlias(fqdn: string, data: AddAliasRequest): Promise<SiteAlias> {
    const response = await this.client.post<SiteAlias>(`/sites/${fqdn}/aliases`, data);
    return response.data;
  }

  async deleteAlias(fqdn: string, alias: string): Promise<void> {
    await this.client.delete(`/sites/${fqdn}/aliases/${alias}`);
  }

  // Versions endpoints
  async listVersions(fqdn: string, page = 1, pageSize = 25): Promise<PaginatedResponse<Version>> {
    const response = await this.client.get<PaginatedResponse<Version>>(`/sites/${fqdn}/versions`, {
      params: { page, page_size: pageSize },
    });
    return response.data;
  }

  async deployVersion(fqdn: string, versionId: string, data: DeployVersionRequest): Promise<void> {
    await this.client.post(`/sites/${fqdn}/versions/${versionId}/deploy`, data);
  }

  async deleteVersion(fqdn: string, versionId: string): Promise<void> {
    await this.client.delete(`/sites/${fqdn}/versions/${versionId}`);
  }

  async downloadVersion(fqdn: string, versionId: string): Promise<Blob> {
    const response = await this.client.get(`/sites/${fqdn}/versions/${versionId}/download`, {
      responseType: 'blob',
    });
    return response.data;
  }

  // Build endpoint
  async build(fqdn: string, data: BuildRequest): Promise<BuildResponse> {
    // Handle file uploads with multipart/form-data
    if (data.files && data.files.length > 0) {
      const formData = new FormData();
      formData.append('message', data.message);
      if (data.conversation_id) {
        formData.append('conversation_id', data.conversation_id);
      }
      data.files.forEach((file) => {
        formData.append('files', file);
      });

      const response = await this.client.post<BuildResponse>(`/sites/${fqdn}/build`, formData, {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
      });
      return response.data;
    }

    const response = await this.client.post<BuildResponse>(`/sites/${fqdn}/build`, data);
    return response.data;
  }
}

export const apiClient = new ApiClient();
