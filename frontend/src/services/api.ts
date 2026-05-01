import axios from 'axios'

const apiClient = axios.create({
  baseURL: '/api',
  headers: {
    'Content-Type': 'application/json',
  },
})

// Request interceptor to attach JWT token
apiClient.interceptors.request.use((config) => {
  // TODO: Get token from pinia store
  // const token = useAuthStore().token
  // if (token) {
  //   config.headers.Authorization = \`Bearer \${token}\`
  // }
  return config
})

export default apiClient
