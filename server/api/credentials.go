package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/infra/crypto"
)

type storeCredentialReq struct {
	WorkspaceID string `json:"workspace_id"`
	Connector   string `json:"connector"`
	Label       string `json:"label"`
	APIKey      string `json:"api_key"` // plaintext; encrypted at rest if server has encKey
}

func (a *API) storeCredential(w http.ResponseWriter, r *http.Request) {
	var req storeCredentialReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WorkspaceID == "" || req.Connector == "" {
		writeError(w, http.StatusBadRequest, "workspace_id and connector are required")
		return
	}

	var encrypted []byte
	if len(a.encKey) == 32 {
		aes, err := crypto.NewAES(a.encKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encryption setup failed")
			return
		}
		enc, err := aes.Encrypt([]byte(req.APIKey), []byte(req.WorkspaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encryption failed")
			return
		}
		encrypted = enc
	} else {
		// Dev mode: store plaintext
		encrypted = []byte(req.APIKey)
	}

	cred := &store.Credential{
		ID:          uuid.NewString(),
		WorkspaceID: req.WorkspaceID,
		Connector:   req.Connector,
		Label:       req.Label,
		KeyHash:     crypto.Hash(req.APIKey),
		Encrypted:   encrypted,
		CreatedAt:   time.Now().UnixMilli(),
	}

	if err := a.app.StoreCredential(r.Context(), cred); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("store credential: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":        cred.ID,
		"connector": cred.Connector,
		"label":     cred.Label,
		"status":    "stored",
	})
}
