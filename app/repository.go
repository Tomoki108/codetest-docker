package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
)

var errTotalAmountLimitExceeded = errors.New("ユーザーの累計取引金額が上限を超えています")

const totalAmountLimit = 1000 //　1ユーザーの累計取引金額上限

var db *sql.DB

func init() {
	var err error
	db, err = sql.Open("mysql", "root@tcp(127.0.0.1:3306)/testdb")
	if err != nil {
		log.Fatal(err)
	}
}

func createTransaction(ctx context.Context, userID int, amount int, description string) (err error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable, // 競合をより厳格に防ぎたいなら Serializable。MySQL の場合、デフォルトで REPEATABLE READ。
	})
	if err != nil {
		return fmt.Errorf("トランザクション開始失敗: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		}
	}()

	// ユーザーの累計取引額を取得
	var currentTotal int
	getUserQuery := `SELECT total_amount FROM users WHERE user_id = ? FOR UPDATE`
	row := tx.QueryRowContext(ctx, getUserQuery, userID)
	if scanErr := row.Scan(&currentTotal); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			err = fmt.Errorf("ユーザー %d が存在しません: %w", userID, scanErr)
		} else {
			err = fmt.Errorf("累計額取得時にエラー: %w", scanErr)
		}
		return err
	}

	// 累計取引額チェック
	newTotal := currentTotal + amount
	if newTotal > totalAmountLimit {
		return fmt.Errorf("%w: 現在 %d, 新しい %d", errTotalAmountLimitExceeded, currentTotal, newTotal)
	}

	// ユーザーの累計取引額を更新
	updateUserQuery := `UPDATE users SET total_amount = ? WHERE user_id = ?`
	if _, execErr := tx.ExecContext(ctx, updateUserQuery, newTotal, userID); execErr != nil {
		err = fmt.Errorf("users テーブル更新エラー: %w", execErr)
		return err
	}

	// transactions テーブルに新しい取引を挿入
	insertTransactionQuery := `
        INSERT INTO transactions (user_id, amount, description)
        VALUES (?, ?, ?)`
	if _, execErr := tx.ExecContext(ctx, insertTransactionQuery, userID, amount, description); execErr != nil {
		err = fmt.Errorf("transactions 挿入エラー: %w", execErr)
		return err
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("コミットエラー: %w", commitErr)
		return err
	}

	return nil
}
