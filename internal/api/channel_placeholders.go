package api

import (
	"net/http"
)

func (h *ChannelHandler) ConnectTikTok(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"code":    "not_implemented",
		"message": "TikTok integration is coming soon",
	})
}

func (h *ChannelHandler) ExchangeTikTok(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"code":    "not_implemented",
		"message": "TikTok integration is coming soon",
	})
}

func (h *ChannelHandler) ConnectInstagram(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"code":    "not_implemented",
		"message": "Instagram integration is coming soon",
	})
}

func (h *ChannelHandler) ExchangeInstagram(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"code":    "not_implemented",
		"message": "Instagram integration is coming soon",
	})
}

func PlatformValidationMiddleware(next http.Handler) http.Handler {
	supportedPlatforms := map[string]bool{
		"youtube": true,
		"tiktok":  true,
		"instagram": true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		platform := r.URL.Query().Get("platform")
		if platform == "" {
			next.ServeHTTP(w, r)
			return
		}
		if !supportedPlatforms[platform] {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"code":    "unsupported_platform",
				"message": "platform " + platform + " is not supported",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
