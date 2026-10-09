//go:build integration

package integration_test

import (
    "bytes"
    "context"
    "crypto/sha256"
    "fmt"
    "image"
    "image/jpeg"
    "os"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgconn"
    "github.com/jackc/pgx/v5/pgxpool"
    "errors"
)

func TestProductImagesStoreTenantBoundary(t *testing.T) {
    dsn := os.Getenv("TEST_DATABASE_URL")
    if dsn == "" {
        t.Skip("TEST_DATABASE_URL not set")
    }
    ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
    defer cancel()
    pool, err := pgxpool.New(ctx, dsn)
    if err != nil { t.Fatal(err) }
    defer pool.Close()
    tx, err := pool.Begin(ctx)
    if err != nil { t.Fatal(err) }
    defer func() { _ = tx.Rollback(context.Background()) }()

    tenants := [2]string{}
    products := [2]string{}
    suffix := uuid.New().String()
    for i := range tenants {
        cnpj := fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000 + int64(i))
        if err := tx.QueryRow(ctx, `INSERT INTO companies(legal_name, cnpj) VALUES($1, $2) RETURNING id::text`,
            "Product Image Tenant "+suffix, cnpj).Scan(&tenants[i]); err != nil { t.Fatal(err) }
        if err := tx.QueryRow(ctx, `INSERT INTO products(tenant_id, sku, name, unit, price_cash)
                 VALUES($1,$2,'Caderno com cores','un',14.90) RETURNING id::text`,
            tenants[i], fmt.Sprintf("MEDIA-%s-%d", suffix, i)).Scan(&products[i]); err != nil { t.Fatal(err) }
    }

    var encoded bytes.Buffer
    if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 8, 8)),
        &jpeg.Options{Quality: 80}); err != nil { t.Fatal(err) }
    raw := encoded.Bytes()
    hash := sha256.Sum256(raw)
    uploadKey := uuid.New().String()

    var imageID string
    if err := tx.QueryRow(ctx, `INSERT INTO product_images(tenant_id, product_id, upload_key, content_sha256, image_data, thumb_data)
                VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`,
        tenants[0], products[0], uploadKey, hash[:], raw, raw).Scan(&imageID); err != nil { t.Fatal(err) }

    var countA, countB int
    if err := tx.QueryRow(ctx, `SELECT count(*) FROM product_images WHERE tenant_id=$1 AND product_id=$2`,
        tenants[0], products[0]).Scan(&countA); err != nil { t.Fatal(err) }
    if err := tx.QueryRow(ctx, `SELECT count(*) FROM product_images WHERE tenant_id=$1 AND product_id=$2`,
        tenants[1], products[0]).Scan(&countB); err != nil { t.Fatal(err) }
    if countA != 1 || countB != 0 { t.Fatalf("tenant leak: A=%d B=%d", countA, countB) }

    // Same upload key for the same company/product must not create duplicates.
    if _, err := tx.Exec(ctx, "SAVEPOINT media_attempt"); err != nil { t.Fatal(err) }
    _, err = tx.Exec(ctx, `INSERT INTO product_images(tenant_id, product_id, upload_key, content_sha256, image_data, thumb_data)
                          VALUES($1,$2,$3,$4,$5,$6)`,
        tenants[0], products[0], uploadKey, hash[:], raw, raw)
    var pgErr *pgconn.PgError
    if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
        t.Fatalf("duplicate upload must reject: %v", err)
    }
    if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT media_attempt"); err != nil { t.Fatal(err) }

    // Composite FK rejects attempts to attach A's product using B's tenant.
    if _, err := tx.Exec(ctx, "SAVEPOINT cross_company"); err != nil { t.Fatal(err) }
    _, err = tx.Exec(ctx, `INSERT INTO product_images(tenant_id, product_id, upload_key, content_sha256, image_data, thumb_data)
                          VALUES($1,$2,$3,$4,$5,$6)`,
        tenants[1], products[0], uuid.New().String(), hash[:], raw, raw)
    if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
        t.Fatalf("cross-company photo must reject: %v", err)
    }
    if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT cross_company"); err != nil { t.Fatal(err) }

    // Both companies can create their own media with their independent product.
    if _, err := tx.Exec(ctx, `INSERT INTO product_images(tenant_id, product_id, upload_key, content_sha256, image_data, thumb_data)
                          VALUES($1,$2,$3,$4,$5,$6)`,
        tenants[1], products[1], uuid.New().String(), hash[:], raw, raw); err != nil { t.Fatal(err) }
}
