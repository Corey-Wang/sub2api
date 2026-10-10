package service

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const siwcPersonalOnlyExtraKey = "siwc_personal_only"

func (s *adminServiceImpl) validateSIWCGroups(ctx context.Context, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
	}
	if s == nil || s.groupRepo == nil {
		return infraerrors.BadRequest("SIWC_GROUP", "无法校验分组，官方 SIWC 账号不能加入未确认的共享分组")
	}
	seen := map[int64]struct{}{}
	for _, id := range groupIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		group, err := s.groupRepo.GetByID(ctx, id)
		if err != nil || group == nil || !group.IsExclusive {
			return infraerrors.BadRequest("SIWC_SHARED_GROUP", "官方 SIWC 账号仅限本人使用，不能加入非专属分组")
		}
	}
	return nil
}

func applySIWCCreatePolicy(account *Account, ownerUserID int64) error {
	if account == nil {
		return infraerrors.BadRequest("SIWC_ACCOUNT", "missing account")
	}
	if ownerUserID <= 0 {
		ownerUserID = siwcOwnerUserID(account)
	}
	if ownerUserID <= 0 {
		return infraerrors.BadRequest("SIWC_OWNER_REQUIRED", "官方 SIWC 账号必须绑定创建它的管理员")
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[siwcPersonalOnlyExtraKey] = true
	if account.Credentials == nil {
		account.Credentials = map[string]any{}
	}
	account.Credentials["owner_user_id"] = ownerUserID
	return nil
}

func preserveSIWCOwner(account *Account, previous any) error {
	if account == nil || account.Type != AccountTypeSIWC {
		return nil
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[siwcPersonalOnlyExtraKey] = true
	if account.Credentials == nil {
		account.Credentials = map[string]any{}
	}
	if previous != nil {
		account.Credentials["owner_user_id"] = previous
	}
	if siwcOwnerUserID(account) <= 0 {
		return infraerrors.BadRequest("SIWC_OWNER_REQUIRED", "官方 SIWC 账号必须绑定创建它的管理员")
	}
	return nil
}
