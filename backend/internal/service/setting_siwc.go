package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
	"github.com/google/uuid"
)

var siwcHostIDMu sync.Mutex

// GetOrCreateSIWCHostID returns the instance-stable ext_agent_host_id.
// It is not a secret. OpenAI wants one id per host, created before the first sign-in.
func (s *SettingService) GetOrCreateSIWCHostID(ctx context.Context) (string, error) {
	if s == nil || s.settingRepo == nil {
		return "", errors.New("siwc host id settings are not configured")
	}
	siwcHostIDMu.Lock()
	defer siwcHostIDMu.Unlock()
	value, err := s.settingRepo.GetValue(ctx, siwc.HostIDSettingKey)
	if err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), nil
	}
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return "", err
	}
	id := "urn:uuid:" + uuid.NewString()
	if err := s.settingRepo.Set(ctx, siwc.HostIDSettingKey, id); err != nil {
		return "", err
	}
	stored, err := s.settingRepo.GetValue(ctx, siwc.HostIDSettingKey)
	if err == nil && strings.TrimSpace(stored) != "" {
		return strings.TrimSpace(stored), nil
	}
	return id, nil
}
