import apiClient from './../client';
import qs from 'qs';
import {FullMessageInfoDto, MessageDictionariesDto, MessageFinderResponseDto, MessageSendResponse} from './message.dto';

export const messageService = {
    async getTransports() {
        return await apiClient.get('/api/v1/transports');
    },
    async getMessages(page = 1, perPage = 10, filters = {}) {
        const cleanedFilters = {};
        Object.keys(filters).forEach(key => {
            if (Array.isArray(filters[key]) && filters[key].length > 0) {
                cleanedFilters[key] = filters[key];
            } else if (filters[key] && !Array.isArray(filters[key])) {
                cleanedFilters[key] = filters[key];
            }
        });

        const responseData = await apiClient.get('/api/admin/v1/message', {
            params: {
                page,
                per_page: perPage,
                ...cleanedFilters
            },
            paramsSerializer: params => {
                return qs.stringify(params, { arrayFormat: 'repeat' });
            }
        });

        return new MessageFinderResponseDto(responseData);
    },

    async getMessageById(uuid) {
        const responseData = await apiClient.get(`/api/admin/v1/message/${uuid}`);

        return new FullMessageInfoDto(responseData);
    },

    async getMessageDictionaries() {
        const responseData = await apiClient.get('/api/admin/v1/dictionary/message');
        return new MessageDictionariesDto(responseData);
    },

    async getRecipientSuggestions(searchQuery = '') {
        const responseData = await apiClient.get('/api/admin/v1/dictionary/message-recipients', {
            params: { search: searchQuery }
        });
        return Array.isArray(responseData) ? responseData : [];
    },

    async sendNewMessage(messagePayload) {
        const responseData = await apiClient.post('/api/v1/send', messagePayload);
        return new MessageSendResponse(responseData);
    },
};
