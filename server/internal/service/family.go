// Package service：家庭组相关业务。
package service

import (
	"context"

	"familypoints/server/internal/model"
	"familypoints/server/internal/repo"
)

// Family 是家庭组服务。
type Family struct {
	families *repo.Family
}

// NewFamily 构造。
func NewFamily(r *repo.Family) *Family { return &Family{families: r} }

// MemberBrief 是对外暴露的成员信息：**绝不包含 pass_hash**。
type MemberBrief struct {
	ID          uint64  `json:"id"`
	Name        string  `json:"name"`
	Role        string  `json:"role"`
	Avatar      *string `json:"avatar"`
	Score       int     `json:"score"`
	FrozenScore int     `json:"frozen_score"`
	Hidden      bool    `json:"hidden,omitempty"`
}

// Info 返回家庭组基本信息。
//
// 对成员暴露的邀请码仅在仍有效时返回，避免把过期码给出去造成误导。
func (s *Family) Info(ctx context.Context, familyID uint64) (*model.Family, error) {
	return s.families.ByID(ctx, familyID)
}

// Members 返回组成员。includeHidden 仅后台可传 true。
func (s *Family) Members(ctx context.Context, familyID uint64, includeHidden bool) ([]MemberBrief, error) {
	// 组不存在时返回 404，而不是空列表——空列表会让人以为「组在但没人」
	if _, err := s.families.ByID(ctx, familyID); err != nil {
		return nil, err
	}
	list, err := s.families.Members(ctx, familyID, includeHidden)
	if err != nil {
		return nil, err
	}
	out := make([]MemberBrief, 0, len(list))
	for _, m := range list {
		out = append(out, MemberBrief{
			ID: m.ID, Name: m.Name, Role: m.Role, Avatar: m.Avatar,
			Score: m.Score, FrozenScore: m.FrozenScore, Hidden: m.Hidden,
		})
	}
	return out, nil
}
