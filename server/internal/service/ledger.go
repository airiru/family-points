// Package service：账目业务（商品、兑换、赠送、流水）。
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"familypoints/server/internal/model"
	"familypoints/server/internal/notify"
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
	notifier *notify.Service
	ledger   *repo.Ledger
	families *repo.Family
}

// NewLedger 构造。
func NewLedger(l *repo.Ledger, f *repo.Family, n *notify.Service) *Ledger {
	return &Ledger{ledger: l, families: f, notifier: n}
}

// ---------- 商品 ----------

// Items 列出本组商品。
func (s *Ledger) Items(ctx context.Context, familyID uint64) ([]model.Item, error) {
	return s.ledger.Items(ctx, familyID)
}

// ItemView 是成员端看到的商品：**能不能换由后端算**，前端只展示。
//
// 为什么不让前端比 `score >= cost`：余额是缓存值，两边各算一次必然出现
// 「界面显示可兑换、点了却余额不足」。判定权统一在后端，前端只读这个字段。
type ItemView struct {
	ID         uint64 `json:"id"`
	Name       string `json:"name"`
	Cost       int    `json:"cost"`
	Affordable bool   `json:"affordable"` // 当前可用积分是否够兑换
}

// ItemsFor 列出商品，并按**当前**成员的可用积分标注是否可兑换。
// me 来自中间件（每请求从库里查，分数是新鲜的）。
func (s *Ledger) ItemsFor(ctx context.Context, me *model.Member) ([]ItemView, error) {
	items, err := s.ledger.Items(ctx, *me.FamilyID)
	if err != nil {
		return nil, err
	}
	out := make([]ItemView, 0, len(items))
	for _, it := range items {
		out = append(out, ItemView{
			ID: it.ID, Name: it.Name, Cost: it.Cost,
			Affordable: me.Score >= it.Cost,
		})
	}
	return out, nil
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
	// 受赠方收到通知（旁路，失败不影响转账）
	s.notifier.TransferIn(ctx, *me.FamilyID, toID, amount, me.Name)
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
