package agent

import (
	"time"

	"github.com/wandxy/hand/internal/config"
	models "github.com/wandxy/hand/internal/model"
	storage "github.com/wandxy/hand/internal/state/core"
	statemanager "github.com/wandxy/hand/internal/state/manager"
)

// StateStoreOpener opens the durable state store used by the agent.
type StateStoreOpener func(*config.Config, models.Client) (storage.Store, error)

// StateManagerFactory wraps state manager construction so tests can inject
// controlled stores and expiry settings.
type StateManagerFactory func(storage.Store, time.Duration, time.Duration) (*statemanager.Manager, error)

// OpenStateStore is the production state-store opener used by Agent startup.
var OpenStateStore StateStoreOpener = statemanager.OpenStoreWithRerankerClient

// NewStateManager is the production state manager constructor used by Agent startup.
var NewStateManager StateManagerFactory = statemanager.NewManager
