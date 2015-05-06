package volumemanager

import (
	"encoding/json"

	"github.com/lmars/flynn-webhook-deploy/Godeps/_workspace/src/github.com/flynn/flynn/host/volume"
	"github.com/lmars/flynn-webhook-deploy/Godeps/_workspace/src/github.com/flynn/flynn/host/volume/zfs"
)

func NewProvider(pspec *volume.ProviderSpec) (provider volume.Provider, err error) {
	switch pspec.Kind {
	case "zfs":
		config := &zfs.ProviderConfig{}
		if err := json.Unmarshal(pspec.Config, config); err != nil {
			return nil, err
		}
		if provider, err = zfs.NewProvider(config); err != nil {
			return
		}
		return
	default:
		return nil, volume.UnknownProviderKind
	}
}
