package cache

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Cache stores LLM responses keyed by diff hash to avoid re-reviewing identical code.
type Cache struct {
	db  *sql.DB
	ttl time.Duration
}

// New creates or opens a cache database.
func New(dbPath string, ttl time.Duration) (*Cache, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS llm_cache (
		key TEXT PRIMARY KEY,
		response TEXT NOT NULL,
		model TEXT NOT NULL,
		tokens_in INTEGER NOT NULL DEFAULT 0,
		tokens_out INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS usage_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		model TEXT NOT NULL,
		tokens_in INTEGER NOT NULL DEFAULT 0,
		tokens_out INTEGER NOT NULL DEFAULT 0,
		cost_usd REAL NOT NULL DEFAULT 0,
		cached INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}

	if ttl == 0 {
		ttl = 24 * time.Hour
	}

	return &Cache{db: db, ttl: ttl}, nil
}

// Close closes the cache.
func (c *Cache) Close() error {
	return c.db.Close()
}

// DiffHash computes a cache key from diff content + model + system prompt hash.
func DiffHash(diffContent, model, promptHash string) string {
	h := sha256.Sum256([]byte(diffContent + "|" + model + "|" + promptHash))
	return fmt.Sprintf("%x", h[:16])
}

// Get retrieves a cached response. Returns ("", false) on miss.
func (c *Cache) Get(key string) (string, bool) {
	var response string
	var createdAt time.Time

	err := c.db.QueryRow(
		`SELECT response, created_at FROM llm_cache WHERE key = ?`, key,
	).Scan(&response, &createdAt)

	if err != nil {
		return "", false
	}

	// Check TTL
	if time.Since(createdAt) > c.ttl {
		c.db.Exec(`DELETE FROM llm_cache WHERE key = ?`, key)
		return "", false
	}

	return response, true
}

// Put stores a response in the cache.
func (c *Cache) Put(key, response, model string, tokensIn, tokensOut int) error {
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO llm_cache (key, response, model, tokens_in, tokens_out) VALUES (?, ?, ?, ?, ?)`,
		key, response, model, tokensIn, tokensOut,
	)
	return err
}

// LogUsage records a single LLM call for cost tracking.
func (c *Cache) LogUsage(model string, tokensIn, tokensOut int, costUSD float64, cached bool) error {
	cachedInt := 0
	if cached {
		cachedInt = 1
	}
	_, err := c.db.Exec(
		`INSERT INTO usage_log (model, tokens_in, tokens_out, cost_usd, cached) VALUES (?, ?, ?, ?, ?)`,
		model, tokensIn, tokensOut, costUSD, cachedInt,
	)
	return err
}

// UsageStats holds aggregated usage statistics.
type UsageStats struct {
	TotalCalls     int
	CachedCalls    int
	TotalTokensIn  int
	TotalTokensOut int
	TotalCostUSD   float64
	Period         string
}

// GetStats returns usage statistics for a given period.
func (c *Cache) GetStats(since time.Time) (*UsageStats, error) {
	stats := &UsageStats{}

	err := c.db.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN cached = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(tokens_in), 0),
			COALESCE(SUM(tokens_out), 0),
			COALESCE(SUM(cost_usd), 0)
		FROM usage_log WHERE created_at >= ?
	`, since).Scan(&stats.TotalCalls, &stats.CachedCalls, &stats.TotalTokensIn, &stats.TotalTokensOut, &stats.TotalCostUSD)

	return stats, err
}

// ClearCache removes all cached responses.
func (c *Cache) ClearCache() error {
	_, err := c.db.Exec(`DELETE FROM llm_cache`)
	return err
}

// CacheSize returns the number of cached entries.
func (c *Cache) CacheSize() (int, error) {
	var count int
	err := c.db.QueryRow(`SELECT COUNT(*) FROM llm_cache`).Scan(&count)
	return count, err
}
