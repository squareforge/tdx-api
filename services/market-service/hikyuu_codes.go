package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/glebarez/go-sqlite"
	"github.com/injoyai/tdx"
)

func hikyuuStockDatabasePath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("HIKYUU_STOCK_DB")); configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("HIKYUU_STOCK_DB不可用: %w", err)
		}
		return configured, nil
	}
	for _, candidate := range []string{
		"/app/data/hikyuu/stocks/stock.db",
		filepath.Join("data", "hikyuu", "stocks", "stock.db"),
		filepath.Join("..", "..", "data", "hikyuu", "stocks", "stock.db"),
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("未找到Hikyuu股票代码库stock.db")
}

func loadHikyuuCodeModels(ctx context.Context) ([]*tdx.CodeModel, error) {
	path, err := hikyuuStockDatabasePath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	rows, err := db.QueryContext(ctx, `SELECT s.code,s.name,LOWER(m.market)
		FROM Stock s JOIN Market m ON m.marketid=s.marketid
		WHERE s.valid=1 AND s.type IN (1,8,9,11) AND LOWER(m.market) IN ('sh','sz','bj')
		ORDER BY s.code`)
	if err != nil {
		return nil, fmt.Errorf("读取Hikyuu股票代码失败: %w", err)
	}
	defer rows.Close()
	models := make([]*tdx.CodeModel, 0, 5000)
	for rows.Next() {
		var code, name, exchange string
		if err := rows.Scan(&code, &name, &exchange); err != nil {
			return nil, err
		}
		models = append(models, &tdx.CodeModel{Code: code, Name: name, Exchange: exchange})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, errors.New("Hikyuu股票代码表为空")
	}
	return models, nil
}
