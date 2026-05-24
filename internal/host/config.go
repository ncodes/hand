package host

import (
	"time"

	"github.com/wandxy/hand/internal/config"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
	models "github.com/wandxy/hand/pkg/agent/model"
)

type StateStoreOpener func(*config.Config, models.Client) (storage.Store, error)

type StateManagerFactory func(storage.Store, time.Duration, time.Duration) (*statemanager.Manager, error)

var OpenStateStore StateStoreOpener = statemanager.OpenStoreWithRerankerClient

var NewStateManager StateManagerFactory = statemanager.NewManager
