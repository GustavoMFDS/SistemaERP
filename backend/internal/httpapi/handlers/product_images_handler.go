package handlers

import (
    "bytes"
    "crypto/sha256"
    "encoding/base64"
    "errors"
    "image/jpeg"
    "net/http"
    "unicode/utf8"
    "strings"

    "github.com/example/sistemaemgo/internal/httpapi/middleware"
    "github.com/example/sistemaemgo/internal/modules/audit"
    "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

const (
    maxCatalogPhotos = 5
    maxCatalogPhotoBytes = 256 * 1024
    maxCatalogThumbBytes = 12 * 1024
)

// ProductImagesHandler stores only bounded, re-encoded JPEGs. It does not
// expose public static URLs or allow one independent company to read another's media.
type ProductImagesHandler struct {
    pool *pgxpool.Pool
    audit *audit.Service
}

func NewProductImagesHandler(pool *pgxpool.Pool, a *audit.Service) *ProductImagesHandler {
    return &ProductImagesHandler{pool: pool, audit: a}
}

type productImage struct {
    ID string `json:"id"`
    DataURL string `json:"data_url"`
    ThumbnailURL string `json:"thumbnail_url"`
    Principal bool `json:"principal"`
    Caption string `json:"caption"`
}

func photoError(w http.ResponseWriter, r *http.Request, status int, msg string) {
    writeError(w, r, status, errorCodeForStatus(status), msg, nil)
}

func catalogJPEG(encoded string, maxBytes, maxDimension int) ([]byte, error) {
    if len(encoded) == 0 || len(encoded) > base64.StdEncoding.EncodedLen(maxBytes) {
        return nil, ErrValidation
    }
    raw, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil || len(raw) == 0 || len(raw) > maxBytes {
        return nil, ErrValidation
    }
    if http.DetectContentType(raw) != "image/jpeg" {
        return nil, ErrValidation
    }
    config, err := jpeg.DecodeConfig(bytes.NewReader(raw))
    if err != nil || config.Width < 1 || config.Height < 1 ||
        config.Width > maxDimension || config.Height > maxDimension {
        return nil, ErrValidation
    }
    decoded, err := jpeg.Decode(bytes.NewReader(raw))
    if err != nil {
        return nil, ErrValidation
    }
    // Never trust arbitrary JPEG metadata from clients: decode and recompress
    // server-side so the stored payload is image pixels, not an upload polyglot.
    for _, quality := range []int{85, 72, 58, 42} {
        var normalized bytes.Buffer
        if err := jpeg.Encode(&normalized, decoded, &jpeg.Options{Quality: quality}); err != nil {
            return nil, ErrValidation
        }
        if normalized.Len() <= maxBytes {
            return normalized.Bytes(), nil
        }
    }
    return nil, ErrValidation
}

func catalogProductAndTenant(r *http.Request) (tenantID, productID, actorID string, valid bool) {
    user, ok := middleware.GetAuthUser(r.Context())
    if !ok {
        return "", "", "", false
    }
    id, err := uuid.Parse(chi.URLParam(r, "id"))
    if err != nil || id == uuid.Nil {
        return "", "", "", false
    }
    return user.TenantID, id.String(), user.UserID, true
}

func (h *ProductImagesHandler) List(w http.ResponseWriter, r *http.Request) {
    tenantID, productID, _, valid := catalogProductAndTenant(r)
    if !valid {
        photoError(w, r, http.StatusBadRequest, "Produto inválido")
        return
    }
    var exists bool
    if err := h.pool.QueryRow(r.Context(),
        "SELECT EXISTS(SELECT 1 FROM products WHERE tenant_id=$1 AND id=$2)",
        tenantID, productID).Scan(&exists); err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível consultar o produto")
        return
    }
    if !exists {
        photoError(w, r, http.StatusNotFound, "Produto não encontrado nesta loja")
        return
    }
    rows, err := h.pool.Query(r.Context(), `
        SELECT id::text, encode(image_data,'base64'), encode(thumb_data,'base64'), caption
        FROM product_images WHERE tenant_id=$1 AND product_id=$2
        ORDER BY created_at, id
    `, tenantID, productID)
    if err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível consultar as fotos")
        return
    }
    photos := make([]productImage, 0, maxCatalogPhotos)
    for rows.Next() {
        var item productImage
        var imageText, thumbText string
        if err := rows.Scan(&item.ID, &imageText, &thumbText, &item.Caption); err != nil {
            rows.Close()
            photoError(w, r, http.StatusInternalServerError, "Não foi possível ler as fotos")
            return
        }
        item.Principal = len(photos) == 0
        item.DataURL = "data:image/jpeg;base64," + imageText
        item.ThumbnailURL = "data:image/jpeg;base64," + thumbText
        photos = append(photos, item)
    }
    err = rows.Err()
    rows.Close()
    if err != nil {
        photoError(w, r, http.StatusInternalServerError, "Falha na leitura das fotos")
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"items": photos})
}

// Previews supplies tiny tenant-scoped thumbnails in one request.
// It never transfers original images to the PDV search or inventory lists.
func (h *ProductImagesHandler) Previews(w http.ResponseWriter, r *http.Request) {
    user, ok := middleware.GetAuthUser(r.Context())
    if !ok {
        photoError(w, r, http.StatusUnauthorized, "Sessão inválida")
        return
    }
    rawIDs := r.URL.Query()["id"]
    if len(rawIDs) == 0 {
        writeJSON(w, http.StatusOK, map[string]any{"items": map[string]string{}})
        return
    }
    if len(rawIDs) > 200 {
        photoError(w, r, http.StatusUnprocessableEntity, "Selecione até 200 produtos")
        return
    }
    ids := make([]string, 0, len(rawIDs))
    for _, raw := range rawIDs {
        id, err := uuid.Parse(strings.TrimSpace(raw))
        if err != nil || id == uuid.Nil {
            photoError(w, r, http.StatusUnprocessableEntity, "Lista de produtos inválida")
            return
        }
        ids = append(ids, id.String())
    }
    rows, err := h.pool.Query(r.Context(), `
        SELECT DISTINCT ON (product_id) product_id::text, encode(thumb_data,'base64')
        FROM product_images
        WHERE tenant_id=$1 AND product_id = ANY($2::uuid[])
        ORDER BY product_id, created_at, id
    `, user.TenantID, ids)
    if err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível consultar miniaturas")
        return
    }
    out := map[string]string{}
    for rows.Next() {
        var id, encoded string
        if err := rows.Scan(&id, &encoded); err != nil {
            rows.Close()
            photoError(w, r, http.StatusInternalServerError, "Erro ao ler miniaturas")
            return
        }
        out[id] = "data:image/jpeg;base64," + encoded
    }
    err = rows.Err()
    rows.Close()
    if err != nil {
        photoError(w, r, http.StatusInternalServerError, "Erro ao consultar miniaturas")
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *ProductImagesHandler) Upload(w http.ResponseWriter, r *http.Request) {
    tenantID, productID, actorID, valid := catalogProductAndTenant(r)
    if !valid {
        photoError(w, r, http.StatusBadRequest, "Produto inválido")
        return
    }
    key, err := uuid.Parse(strings.TrimSpace(r.Header.Get("Idempotency-Key")))
    if err != nil || key == uuid.Nil {
        photoError(w, r, http.StatusUnprocessableEntity, "Não foi possível identificar a tentativa de envio")
        return
    }
    var req struct {
        ImageBase64 string `json:"image_base64"`
        ThumbnailBase64 string `json:"thumbnail_base64"`
    }
    if err := readJSON(w, r, &req); err != nil {
        photoError(w, r, http.StatusBadRequest, "Arquivo de imagem inválido")
        return
    }
    raw, err := catalogJPEG(req.ImageBase64, maxCatalogPhotoBytes, 1600)
    if err != nil {
        photoError(w, r, http.StatusUnprocessableEntity, "Foto inválida ou muito grande")
        return
    }
    thumb, err := catalogJPEG(req.ThumbnailBase64, maxCatalogThumbBytes, 160)
    if err != nil {
        photoError(w, r, http.StatusUnprocessableEntity, "Miniatura inválida ou muito grande")
        return
    }
    digest := sha256.Sum256(raw)
    tx, err := h.pool.Begin(r.Context())
    if err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível salvar a foto")
        return
    }
    defer func() { _ = tx.Rollback(r.Context()) }()
    var lockedID string
    if err = tx.QueryRow(r.Context(), `
        SELECT id::text FROM products WHERE tenant_id=$1 AND id=$2 FOR UPDATE
    `, tenantID, productID).Scan(&lockedID); err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            photoError(w, r, http.StatusNotFound, "Produto não encontrado nesta loja")
        } else {
            photoError(w, r, http.StatusInternalServerError, "Não foi possível acessar o produto")
        }
        return
    }
    var existingID string
    var existingHash []byte
    err = tx.QueryRow(r.Context(), `
        SELECT id::text, content_sha256 FROM product_images
        WHERE tenant_id=$1 AND product_id=$2 AND upload_key=$3
    `, tenantID, productID, key).Scan(&existingID, &existingHash)
    if err == nil {
        if !bytes.Equal(existingHash, digest[:]) {
            photoError(w, r, http.StatusConflict, "Esta tentativa foi usada para outra foto")
            return
        }
        writeJSON(w, http.StatusOK, map[string]any{"id": existingID, "replayed": true})
        return
    }
    if !errors.Is(err, pgx.ErrNoRows) {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível conferir o envio anterior")
        return
    }
    var count int
    if err = tx.QueryRow(r.Context(), `
        SELECT count(*) FROM product_images WHERE tenant_id=$1 AND product_id=$2
    `, tenantID, productID).Scan(&count); err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível contar as fotos")
        return
    }
    if count >= maxCatalogPhotos {
        photoError(w, r, http.StatusConflict, "Este produto já tem cinco fotos")
        return
    }
    var newID string
    if err = tx.QueryRow(r.Context(), `
        INSERT INTO product_images(tenant_id,product_id,upload_key,content_sha256,media_type,image_data,thumb_data)
        VALUES($1,$2,$3,$4,'image/jpeg',$5,$6)
        RETURNING id::text
    `, tenantID, productID, key, digest[:], raw, thumb).Scan(&newID); err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível salvar a foto")
        return
    }
    if err = h.audit.RecordTx(r.Context(), tx, audit.Event{
        TenantID: tenantID, ActorUserID: actorID, Action: "product.image.add",
        ResourceType: "product", ResourceID: productID, Outcome: "success",
        Metadata: map[string]any{"image_id": newID, "size_bytes": len(raw)},
    }); err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível registrar a alteração")
        return
    }
    if err = tx.Commit(r.Context()); err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível confirmar a foto")
        return
    }
    writeJSON(w, http.StatusCreated, map[string]any{"id": newID, "replayed": false})
}

func (h *ProductImagesHandler) Delete(w http.ResponseWriter, r *http.Request) {
    tenantID, productID, actorID, valid := catalogProductAndTenant(r)
    if !valid {
        photoError(w, r, http.StatusBadRequest, "Produto inválido")
        return
    }
    photoID, err := uuid.Parse(chi.URLParam(r, "photoID"))
    if err != nil || photoID == uuid.Nil {
        photoError(w, r, http.StatusUnprocessableEntity, "Foto inválida")
        return
    }
    tx, err := h.pool.Begin(r.Context())
    if err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível excluir a foto")
        return
    }
    defer func() { _ = tx.Rollback(r.Context()) }()
    var lockedID string
    if err = tx.QueryRow(r.Context(), `
        SELECT id::text FROM products WHERE tenant_id=$1 AND id=$2 FOR UPDATE
    `, tenantID, productID).Scan(&lockedID); err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            photoError(w, r, http.StatusNotFound, "Produto não encontrado")
        } else {
            photoError(w, r, http.StatusInternalServerError, "Não foi possível abrir o produto")
        }
        return
    }
    var deletedID string
    err = tx.QueryRow(r.Context(), `
        DELETE FROM product_images WHERE tenant_id=$1 AND product_id=$2 AND id=$3
        RETURNING id::text
    `, tenantID, productID, photoID).Scan(&deletedID)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            photoError(w, r, http.StatusNotFound, "Foto não encontrada nesta loja")
        } else {
            photoError(w, r, http.StatusInternalServerError, "Não foi possível excluir a foto")
        }
        return
    }
    if err = h.audit.RecordTx(r.Context(), tx, audit.Event{
        TenantID: tenantID, ActorUserID: actorID, Action: "product.image.delete",
        ResourceType: "product", ResourceID: productID, Outcome: "success",
        Metadata: map[string]any{"image_id": deletedID},
    }); err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível auditar a exclusão")
        return
    }
    if err = tx.Commit(r.Context()); err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível confirmar a exclusão")
        return
    }
    w.WriteHeader(http.StatusNoContent)
}

func (h *ProductImagesHandler) SetCaption(w http.ResponseWriter, r *http.Request) {
    tenantID, productID, actorID, valid := catalogProductAndTenant(r)
    if !valid {
        photoError(w, r, http.StatusBadRequest, "Produto inválido")
        return
    }
    photoID, err := uuid.Parse(chi.URLParam(r, "photoID"))
    if err != nil || photoID == uuid.Nil {
        photoError(w, r, http.StatusUnprocessableEntity, "Foto inválida")
        return
    }
    var req struct {
        Caption string `json:"caption"`
    }
    if err := readJSON(w, r, &req); err != nil {
        photoError(w, r, http.StatusBadRequest, "Nome da cor inválido")
        return
    }
    caption := strings.TrimSpace(req.Caption)
    if !utf8.ValidString(caption) || utf8.RuneCountInString(caption) > 40 {
        photoError(w, r, http.StatusUnprocessableEntity, "Use até 40 caracteres para identificar a foto")
        return
    }

    tx, err := h.pool.Begin(r.Context())
    if err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível alterar a foto")
        return
    }
    defer func() { _ = tx.Rollback(r.Context()) }()
    // Lock the parent product just like uploads/removals so image operations
    // for a single tenant/product cannot race with each other.
    var id string
    err = tx.QueryRow(r.Context(),
        "SELECT id::text FROM products WHERE tenant_id=$1 AND id=$2 FOR UPDATE",
        tenantID, productID).Scan(&id)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            photoError(w, r, http.StatusNotFound, "Produto não encontrado nesta loja")
        } else {
            photoError(w, r, http.StatusInternalServerError, "Não foi possível acessar o produto")
        }
        return
    }
    err = tx.QueryRow(r.Context(), `
        UPDATE product_images SET caption=$4
        WHERE tenant_id=$1 AND product_id=$2 AND id=$3
        RETURNING id::text
    `, tenantID, productID, photoID, caption).Scan(&id)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            photoError(w, r, http.StatusNotFound, "Foto não encontrada nesta loja")
        } else {
            photoError(w, r, http.StatusInternalServerError, "Não foi possível alterar a legenda")
        }
        return
    }
    if err := h.audit.RecordTx(r.Context(), tx, audit.Event{
        TenantID: tenantID, ActorUserID: actorID,
        Action: "product.image.caption",
        ResourceType: "product", ResourceID: productID, Outcome: "success",
        Metadata: map[string]any{"image_id": id},
    }); err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível registrar a alteração")
        return
    }
    if err := tx.Commit(r.Context()); err != nil {
        photoError(w, r, http.StatusInternalServerError, "Não foi possível confirmar a legenda")
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"id": id, "caption": caption})
}
