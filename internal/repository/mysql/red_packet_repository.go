package mysql

import (
	"context"
	"errors"
	"math/rand"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"IM_Chat_System/internal/model"
	"IM_Chat_System/internal/repository"
)

type redeemCodeRow struct {
	Code      string     `gorm:"column:code;primaryKey"`
	Amount    int64      `gorm:"column:amount"`
	UsedBy    *int64     `gorm:"column:used_by"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (redeemCodeRow) TableName() string { return "redeem_codes" }

type redPacketRow struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	SenderID        int64     `gorm:"column:sender_id"`
	GroupID         *int64    `gorm:"column:group_id"`
	ReceiverID      *int64    `gorm:"column:receiver_id"`
	TotalAmount     int64     `gorm:"column:total_amount"`
	TotalCount      int       `gorm:"column:total_count"`
	RemainingAmount int64     `gorm:"column:remaining_amount"`
	RemainingCount  int       `gorm:"column:remaining_count"`
	Lucky           bool      `gorm:"column:lucky"`
	Greeting        string    `gorm:"column:greeting"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (redPacketRow) TableName() string { return "red_packets" }

func (r redPacketRow) model() model.RedPacket {
	return model.RedPacket{
		ID:              r.ID,
		SenderID:        r.SenderID,
		GroupID:         r.GroupID,
		ReceiverID:      r.ReceiverID,
		TotalAmount:     r.TotalAmount,
		TotalCount:      r.TotalCount,
		RemainingAmount: r.RemainingAmount,
		RemainingCount:  r.RemainingCount,
		Lucky:           r.Lucky,
		Greeting:        r.Greeting,
		CreatedAt:       r.CreatedAt,
	}
}

type redPacketReceiptRow struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	PacketID  int64     `gorm:"column:packet_id"`
	UserID    int64     `gorm:"column:user_id"`
	Amount    int64     `gorm:"column:amount"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (redPacketReceiptRow) TableName() string { return "red_packet_receipts" }

func (r redPacketReceiptRow) model() model.RedPacketReceipt {
	return model.RedPacketReceipt{
		ID:        r.ID,
		PacketID:  r.PacketID,
		UserID:    r.UserID,
		Amount:    r.Amount,
		CreatedAt: r.CreatedAt,
	}
}

type RedPacketRepository struct {
	db *gorm.DB
}

func NewRedPacketRepository(db *gorm.DB) *RedPacketRepository {
	return &RedPacketRepository{db: db}
}

func (r *RedPacketRepository) GetBalance(ctx context.Context, userID int64) (int64, error) {
	var row userRow
	if err := r.db.WithContext(ctx).Select("balance").Where("id = ?", userID).First(&row).Error; err != nil {
		return 0, err
	}
	return row.Balance, nil
}

// RedeemCode 原子兑换：仅当兑换码尚未被使用时标记为已使用并加余额。
func (r *RedPacketRepository) RedeemCode(ctx context.Context, userID int64, code string) (int64, error) {
	var amount int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		res := tx.Model(&redeemCodeRow{}).
			Where("code = ? AND used_at IS NULL", code).
			Updates(map[string]any{"used_by": userID, "used_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return repository.ErrRedeemCodeInvalid
		}
		var row redeemCodeRow
		if err := tx.Where("code = ?", code).First(&row).Error; err != nil {
			return err
		}
		amount = row.Amount
		return tx.Model(&userRow{}).Where("id = ?", userID).
			Update("balance", gorm.Expr("balance + ?", amount)).Error
	})
	if err != nil {
		return 0, err
	}
	return amount, nil
}

// CreatePacket 原子扣除发送者余额并创建红包。
func (r *RedPacketRepository) CreatePacket(ctx context.Context, senderID int64, groupID, receiverID *int64, totalAmount int64, count int, lucky bool, greeting string) (model.RedPacket, error) {
	var packet redPacketRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&userRow{}).
			Where("id = ? AND balance >= ?", senderID, totalAmount).
			Update("balance", gorm.Expr("balance - ?", totalAmount))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return repository.ErrInsufficientBalance
		}
		now := time.Now()
		packet = redPacketRow{
			SenderID:        senderID,
			GroupID:         groupID,
			ReceiverID:      receiverID,
			TotalAmount:     totalAmount,
			TotalCount:      count,
			RemainingAmount: totalAmount,
			RemainingCount:  count,
			Lucky:           lucky,
			Greeting:        greeting,
			CreatedAt:       now,
		}
		return tx.Create(&packet).Error
	})
	if err != nil {
		return model.RedPacket{}, err
	}
	return packet.model(), nil
}

func (r *RedPacketRepository) GetPacket(ctx context.Context, packetID int64) (model.RedPacket, bool, error) {
	var row redPacketRow
	err := r.db.WithContext(ctx).First(&row, packetID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.RedPacket{}, false, nil
	}
	if err != nil {
		return model.RedPacket{}, false, err
	}
	return row.model(), true, nil
}

// Grab 抢红包。核心并发安全：
//   - 事务 + SELECT ... FOR UPDATE 锁红包行，串行化同一红包的并发抢；
//   - 唯一约束 (packet_id, user_id) 兜底防重复抢；
//   - 随机金额用二倍均值法，保证总额分毫不差。
func (r *RedPacketRepository) Grab(ctx context.Context, packetID, userID int64) (model.RedPacketReceipt, error) {
	var receipt redPacketReceiptRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var packet redPacketRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", packetID).First(&packet).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return repository.ErrPacketNotFound
			}
			return err
		}

		var grabbed int64
		if err := tx.Model(&redPacketReceiptRow{}).
			Where("packet_id = ? AND user_id = ?", packetID, userID).
			Count(&grabbed).Error; err != nil {
			return err
		}
		if grabbed > 0 {
			return repository.ErrAlreadyGrabbed
		}
		if packet.RemainingCount <= 0 {
			return repository.ErrPacketEmpty
		}

		amount := randomGrabAmount(packet.RemainingAmount, packet.RemainingCount, packet.Lucky)

		res := tx.Model(&redPacketRow{}).
			Where("id = ? AND remaining_count > 0", packetID).
			Updates(map[string]any{
				"remaining_amount": gorm.Expr("remaining_amount - ?", amount),
				"remaining_count":  gorm.Expr("remaining_count - 1"),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return repository.ErrPacketEmpty
		}

		receipt = redPacketReceiptRow{
			PacketID:  packetID,
			UserID:    userID,
			Amount:    amount,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&receipt).Error; err != nil {
			if isDuplicateKeyError(err) {
				return repository.ErrAlreadyGrabbed
			}
			return err
		}

		return tx.Model(&userRow{}).Where("id = ?", userID).
			Update("balance", gorm.Expr("balance + ?", amount)).Error
	})
	if err != nil {
		return model.RedPacketReceipt{}, err
	}
	return receipt.model(), nil
}

func (r *RedPacketRepository) ListReceipts(ctx context.Context, packetID int64) ([]model.RedPacketReceipt, error) {
	var rows []redPacketReceiptRow
	if err := r.db.WithContext(ctx).Where("packet_id = ?", packetID).
		Order("amount DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	receipts := make([]model.RedPacketReceipt, 0, len(rows))
	for _, row := range rows {
		receipts = append(receipts, row.model())
	}
	return receipts, nil
}

// randomGrabAmount 计算本次抢到的金额（分）。
// 拼手气红包用二倍均值法；普通红包均分，最后一人拿余数。
func randomGrabAmount(remaining int64, count int, lucky bool) int64 {
	if count <= 1 {
		return remaining
	}
	if !lucky {
		return remaining / int64(count)
	}
	avg := remaining / int64(count)
	if avg < 1 {
		avg = 1
	}
	// 上限既不能超过平均值的 2 倍，也要保证剩下 count-1 人每人至少 1 分。
	maxGrab := avg * 2
	if maxGrab > remaining-int64(count-1) {
		maxGrab = remaining - int64(count-1)
	}
	if maxGrab <= 1 {
		return 1
	}
	return 1 + rand.Int63n(maxGrab) // [1, maxGrab]
}

func isDuplicateKeyError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
