package schedulerapi

import (
	"context"
	"fmt"

	"github.com/desain-gratis/common/lib/notifier"
)

var _ StateMachineClient = (*stubSMClient)(nil)

// aka "not-ready" client
type stubSMClient struct{}

func (s *stubSMClient) ID() uint64 {
	return 0
}

func (s *stubSMClient) Propose(ctx context.Context, request any) (any, error) {
	return nil, fmt.Errorf("state machine client is not ready")
}
func (s *stubSMClient) Subscribe(ctx context.Context) (notifier.Subscription, error) {
	return nil, fmt.Errorf("state machine client is not ready")
}
