package service

import "github.com/example/sistemaemgo/internal/repo"

// ProductRepoAdapter exists only to allow wiring in handlers without importing repo there.
// In a production codebase, prefer injecting the repo directly in the handler constructor.

type ProductRepoAdapter struct {
	Repo *repo.ProductsRepo
}
