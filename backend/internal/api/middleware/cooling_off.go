package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type CoolingOffChecker func(userID, txID string) (active bool, remaining time.Duration, err error)

func EnforceCoolingOff(txIDParam string, checker CoolingOffChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if checker == nil {
				next.ServeHTTP(w, r)
				return
			}

			uid, ok := UserIDFromContext(r.Context())
			if !ok || strings.TrimSpace(uid) == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			txID := ""
			if txIDParam != "" {
				txID = strings.TrimSpace(chi.URLParam(r, txIDParam))
			}

			active, remaining, err := checker(uid, txID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if active {
				if remaining > 0 {
					w.Header().Set("Retry-After", strconv.Itoa(int(remaining.Seconds())))
				}
				http.Error(w, "cooling-off lock active", http.StatusLocked)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
