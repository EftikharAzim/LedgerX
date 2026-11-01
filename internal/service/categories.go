package service

import (
	"context"
	"errors"
	"strings"

	"github.com/EftikharAzim/ledgerx/internal/importing"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrCategoryInput identifies invalid or unsafe category organization.
var ErrCategoryInput = errors.New("check the category name, type, icon and parent; only one subcategory level is supported")

// ErrCategoryChanged requires a fresh impact review before deletion.
var ErrCategoryChanged = errors.New("category usage changed; review its impact again")

// ErrCategoryInUse protects child categories and budget scopes from implicit merging.
var ErrCategoryInUse = errors.New("move or delete subcategories and remove this category's budgets before deleting it")

// CategoryService serializes organization changes with financial writes.
type CategoryService struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewCategoryService uses the existing ledger lock and database.
func NewCategoryService(pool *pgxpool.Pool, q *sqlc.Queries) *CategoryService {
	return &CategoryService{pool, q}
}

// CategoryEdit permits partial edits without resetting other preferences.
type CategoryEdit struct {
	Name     *string `json:"name"`
	ParentID *int64  `json:"parent_id"`
	Kind     *string `json:"kind"`
	Icon     *string `json:"icon"`
	Archived *bool   `json:"archived"`
}

// CategoryImpact describes reassignment without changing money.
type CategoryImpact struct {
	Category     sqlc.Category `json:"category"`
	Transactions int64         `json:"transactions"`
	Plans        int64         `json:"plans"`
	Children     int64         `json:"children"`
	Budgets      int64         `json:"budgets"`
	Token        string        `json:"review_token"`
}

func selectableCategory(ctx context.Context, q *sqlc.Queries, uid, id int64) (sqlc.Category, error) {
	c, err := q.GetCategoryForUser(ctx, sqlc.GetCategoryForUserParams{ID: id, UserID: uid})
	if err != nil || c.Archived || c.Deleted {
		return c, ErrInvalidCategory
	}
	if c.ParentID.Valid {
		p, err := q.GetCategoryForUser(ctx, sqlc.GetCategoryForUserParams{ID: c.ParentID.Int64, UserID: uid})
		if err != nil || p.Archived || p.Deleted {
			return c, ErrInvalidCategory
		}
		if c.Kind == "any" {
			c.Kind = p.Kind
		}
	}
	return c, nil
}

func validateEntryCategory(ctx context.Context, q *sqlc.Queries, uid, id, amount int64, entryType string) error {
	c, err := selectableCategory(ctx, q, uid, id)
	if err != nil {
		return err
	}
	kind := "expense"
	if amount > 0 && entryType != "refund" {
		kind = "income"
	}
	if c.Kind != "any" && c.Kind != kind {
		return ErrInvalidCategory
	}
	return nil
}

func categoryImpact(ctx context.Context, tx pgx.Tx, q *sqlc.Queries, uid, id int64) (CategoryImpact, error) {
	c, err := q.GetCategoryForUser(ctx, sqlc.GetCategoryForUserParams{ID: id, UserID: uid})
	if err != nil {
		return CategoryImpact{}, ErrInvalidCategory
	}
	out := CategoryImpact{Category: c}
	err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM transactions WHERE user_id=$1 AND category_id=$2),
 (SELECT count(*) FROM planned_transactions WHERE user_id=$1 AND category_id=$2),
 (SELECT count(*) FROM categories WHERE user_id=$1 AND parent_id=$2 AND NOT deleted),
 (SELECT count(*) FROM budgets WHERE user_id=$1 AND category_id=$2)`, uid, id).Scan(&out.Transactions, &out.Plans, &out.Children, &out.Budgets)
	if err != nil {
		return out, err
	}
	out.Token = importing.Hash(out)
	return out, nil
}

// Impact reads a coherent snapshot of the records affected by deletion.
func (s *CategoryService) Impact(ctx context.Context, uid, id int64) (CategoryImpact, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CategoryImpact{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return CategoryImpact{}, err
	}
	return categoryImpact(ctx, tx, q, uid, id)
}

// Save creates or edits a category, rejecting cycles, foreign parents and third levels.
func (s *CategoryService) Save(ctx context.Context, uid, id int64, in CategoryEdit) (sqlc.Category, error) {
	var c sqlc.Category
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return c, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return c, err
	}
	if id != 0 {
		c, err = q.GetCategoryForUser(ctx, sqlc.GetCategoryForUserParams{ID: id, UserID: uid})
		if err != nil || c.Deleted {
			return c, ErrInvalidCategory
		}
	} else {
		c.Kind = "any"
		c.Icon = "tag"
		c.UserID = uid
	}
	if in.Name != nil {
		c.Name = strings.TrimSpace(*in.Name)
	}
	if in.Kind != nil {
		c.Kind = *in.Kind
	}
	if in.Icon != nil {
		c.Icon = *in.Icon
	}
	if in.ParentID != nil {
		c.ParentID = pgtype.Int8{Int64: *in.ParentID, Valid: *in.ParentID != 0}
	}
	if in.Archived != nil {
		c.Archived = *in.Archived
	}
	icons := map[string]bool{"tag": true, "wallet": true, "home": true, "cart": true, "coffee": true, "car": true, "heart": true, "gift": true, "briefcase": true, "plane": true, "book": true, "food": true}
	if c.Name == "" || len([]rune(c.Name)) > 80 || strings.ContainsRune(c.Name, 0) || !icons[c.Icon] || (c.Kind != "any" && c.Kind != "income" && c.Kind != "expense") || c.ParentID.Int64 < 0 {
		return c, ErrCategoryInput
	}
	if c.ParentID.Valid {
		p, e := q.GetCategoryForUser(ctx, sqlc.GetCategoryForUserParams{ID: c.ParentID.Int64, UserID: uid})
		if e == nil && id == 0 && in.Kind == nil {
			c.Kind = p.Kind
		}
		if e != nil || p.Deleted || p.ParentID.Valid || p.ID == id || (!c.Archived && p.Archived) || (p.Kind != "any" && c.Kind != p.Kind) {
			return c, ErrCategoryInput
		}
		var children bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM categories WHERE parent_id=$1 AND NOT deleted)", id).Scan(&children); err != nil {
			return c, err
		}
		if children {
			return c, ErrCategoryInput
		}
	}
	var incompatible bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM categories WHERE parent_id=$1 AND NOT deleted AND kind<>$2 AND $2<>'any')", id, c.Kind).Scan(&incompatible); err != nil {
		return c, err
	}
	if incompatible {
		return c, ErrCategoryInput
	}
	if c.Kind == "income" {
		var budget bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM budgets b LEFT JOIN categories child ON child.id=b.category_id WHERE b.user_id=$1 AND (b.category_id=$2 OR child.parent_id=$2))", uid, id).Scan(&budget); err != nil {
			return c, err
		}
		if budget {
			return c, ErrCategoryInUse
		}
	}
	if id == 0 {
		created, e := q.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: uid, Name: c.Name})
		if e != nil {
			if isUniqueViolation(e) {
				return c, ErrCategoryInput
			}
			return c, e
		}
		id = created.ID
		c.ID = created.ID
		c.CreatedAt = created.CreatedAt
	}
	_, err = tx.Exec(ctx, "UPDATE categories SET name=$3,parent_id=$4,kind=$5,icon=$6,archived=$7 WHERE user_id=$1 AND id=$2", uid, id, c.Name, c.ParentID, c.Kind, c.Icon, c.Archived)
	if err != nil {
		if isUniqueViolation(err) {
			return c, ErrCategoryInput
		}
		return c, err
	}
	if c.Archived {
		if _, err := tx.Exec(ctx, "UPDATE categories SET archived=true WHERE user_id=$1 AND parent_id=$2 AND NOT deleted", uid, id); err != nil {
			return c, err
		}
	}
	c, err = q.GetCategoryForUser(ctx, sqlc.GetCategoryForUserParams{ID: id, UserID: uid})
	if err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}

// Delete reassigns labels atomically, retaining a historical category identity.
// Financial postings, correction receipts and source fingerprints never change.
func (s *CategoryService) Delete(ctx context.Context, uid, id, replacement int64, token string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return err
	}
	impact, err := categoryImpact(ctx, tx, q, uid, id)
	if err != nil {
		return err
	}
	if impact.Category.Deleted {
		if impact.Category.ReplacementID.Int64 == replacement {
			return nil
		}
		return ErrCategoryChanged
	}
	if token == "" || token != impact.Token {
		return ErrCategoryChanged
	}
	if impact.Children > 0 || impact.Budgets > 0 {
		return ErrCategoryInUse
	}
	if replacement < 0 || replacement == id {
		return ErrInvalidCategory
	}
	if replacement != 0 {
		target, err := selectableCategory(ctx, q, uid, replacement)
		if err != nil {
			return err
		}
		if impact.Category.Kind != "any" && target.Kind != "any" && target.Kind != impact.Category.Kind {
			return ErrCategoryInput
		}
	}
	dest := pgtype.Int8{Int64: replacement, Valid: replacement != 0}
	for _, statement := range []string{
		"UPDATE transactions SET category_id=$3 WHERE user_id=$1 AND category_id=$2",
		"UPDATE transactions SET suggested_category_id=NULL,suggestion_score=NULL WHERE user_id=$1 AND suggested_category_id=$2",
		"UPDATE planned_transactions SET category_id=$3 WHERE user_id=$1 AND category_id=$2",
	} {
		var err error
		if strings.Contains(statement, "$3") {
			_, err = tx.Exec(ctx, statement, uid, id, dest)
		} else {
			_, err = tx.Exec(ctx, statement, uid, id)
		}
		if err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE categories SET deleted=true,archived=true,deleted_at=now(),replacement_id=$3 WHERE user_id=$1 AND id=$2", uid, id, dest); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
