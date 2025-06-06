package main

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
)

var errTotalAmountLimitExceeded = errors.New("ユーザーの累計取引金額が上限を超えています")

const totalAmountLimit = 1000 //　1ユーザーの累計取引金額上限

var db *sql.DB

func init() {
	var err error
	// Docker Compose network: connect to db service by hostname
	db, err = sql.Open("mysql", "root@tcp(db:3306)/codetest")
	if err != nil {
		log.Fatal(err)
	}
}

type Transaction struct {
	UserID      int    `json:"user_id"`
	Amount      int    `json:"amount"`
	Description string `json:"description"`
}

func main() {
	http.HandleFunc("/transactions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req Transaction
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		err := createTransaction(r.Context(), req.UserID, req.Amount, req.Description)
		if errors.Is(err, errTotalAmountLimitExceeded) {
			http.Error(w, err.Error(), http.StatusPaymentRequired)
		} else if err != nil {
			http.Error(w, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})

	if err := http.ListenAndServe(":8888", nil); err != nil {
		log.Println(err)
	}
}

// createTransaction 取引データを登録する
func createTransaction(ctx context.Context, userID int, amount int, description string) (err error) {
	tx, err := db.BeginTx(ctx, nil)
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
	getUserQuery := `SELECT total_amount FROM users WHERE id = ? FOR UPDATE`
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
		return errTotalAmountLimitExceeded
	}

	// ユーザーの累計取引額を更新
	updateUserQuery := `UPDATE users SET total_amount = ? WHERE id = ?`
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
