import axios from 'axios';
import { useConfigStore } from '@/stores/config';

const apiClient = axios.create({timeout: 15000});

apiClient.interceptors.request.use((config) => {
    const configStore = useConfigStore();

    config.baseURL = configStore.host;
    config.headers[configStore.apiHeader] = configStore.apiKey;

    return config;
}, (error) => {
    return Promise.reject(error);
});

apiClient.interceptors.response.use((response) => {
    const serverResponse = response.data;

    if (serverResponse.status === 'error') {
        const errorMessage = serverResponse.error || 'Server processed request with an error';
        return Promise.reject(new Error(errorMessage));
    }

    return serverResponse.data;
}, (error) => {
    const fallbackMessage = error.response?.data?.error || error.message || 'Network error occurred';
    return Promise.reject(new Error(fallbackMessage));
});

export default apiClient;
