// Package service：账目业务（商品、兑换、赠送、流水）。
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"familypoints/server/internal/model"
	"familypoints/server/internal/repo"
)

// 账目相关业务错误。
var (
	ErrInsufficient   = errors.New("积分不足")
	ErrSameMember     = errors.New("不能赠送给自己")
	ErrNotEnoughScore = errors.New("可用余额不足")
)

// Ledger 是账目服务。
type Ledger struct {
	ledger   *repo.Ledger
	families *repo.Family
}

// NewLedger 构造。
func NewLedger(l *repo.Ledger, f *repo.Family) *Ledger {
	return &Ledger{ledger: l, families: f}
}

// ---------- 商品 ----------

// Items 列出本组商品。
func (s *Ledger) Items(ctx context.Context, familyID uint64) ([]model.Item, error) {
	return s.ledger.Items(ctx, familyID)
}

// SaveItem 新增/更新商品（组长）。
func (s *Ledger) SaveItem(ctx context.Context, me *model.Member, id uint64, name string, cost int) (*model.Item, error) {
	if err := requireLeader(me); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return nil, fmt.Errorf("%w：商品名需 1–64 个字符", ErrInvalidInput)
	}
	if cost <= 0 {
		return nil, fmt.Errorf("%w：价格必须是正整数", ErrInvalidInput)
	}
	return s.ledger.SaveItem(ctx, *me.FamilyID, id, name, cost)
}

// DeleteItem 删除商品（组长）。历史兑换单有快照，不受影响。
func (s *Ledger) DeleteItem(ctx context.Context, me *model.Member, id uint64) error {
	if err := requireLeader(me); err != nil {
		return err
	}
	return s.ledger.DeleteItem(ctx, *me.FamilyID, id)
}

// ---------- 兑换 ----------

// Redeem 自助兑换：只能用可用余额（不含冻结积分）。
func (s *Ledger) Redeem(ctx context.Context, me *model.Member, itemID uint64) (*model.Redemption, error) {
	item, err := s.ledger.ItemByID(ctx, *me.FamilyID, itemID)
	if err != nil {
		return nil, err
	}
	rd, err := s.ledger.Redeem(ctx, *me.FamilyID, me.ID, item)
	if err != nil {
		if errors.Is(err, repo.ErrInsufficient) {
			return nil, fmt.Errorf("%w：需要 %d 分，当前可用 %d 分", ErrNotEnoughScore, item.Cost, me.Score)
		}
		return nil, err
	}
	return rd, nil
}

// ---------- 赠送 ----------

// Send 成员赠送积分：只受转出方余额约束，不设单笔/每日上限（19.10）。
func (s *Ledger) Send(ctx context.Context, me *model.Member, toID uint64, amount int, note string) (*model.Transfer, error) {
	if toID == me.ID {
		return nil, ErrSameMember
	}
	if amount <= 0 {
		return nil, fmt.Errorf("%w：金额必须是正整数", ErrInvalidInput)
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 64 {
		return nil, fmt.Errorf("%w：附言最多 64 个字符", ErrInvalidInput)
	}
	t, err := s.ledger.SendTransfer(ctx, *me.FamilyID, me.ID, toID, amount, note)
	if err != nil {
		if errors.Is(err, repo.ErrInsufficient) {
			return nil, fmt.Errorf("%w：当前可用 %d 分", ErrNotEnoughScore, me.Score)
		}
		return nil, err
	}
	return t, nil
}

// Transfers 列出赠送记录。
func (s *Ledger) Transfers(ctx context.Context, familyID uint64, limit int) ([]model.Transfer, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.ledger.Transfers(ctx, familyID, limit)
}

// ---------- 流水 ----------

// RecordPage 是流水分页结果。
type RecordPage struct {
	Items  []model.Record `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// Records 分页查询本组流水。
func (s *Ledger) Records(ctx context.Context, familyID uint64, limit, offset int) (*RecordPage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	list, err := s.ledger.Records(ctx, familyID, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := s.ledger.CountRecords(ctx, familyID)
	if err != nil {
		return nil, err
	}
	return &RecordPage{Items: list, Total: total, Limit: limit, Offset: offset}, nil
}
