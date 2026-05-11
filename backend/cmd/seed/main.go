// Command seed — stub. O usuário administrador é criado pela migration 000007.
// Dados de catálogo são importados via cmd/import-catalog.
package main

import "fmt"

func main() {
	fmt.Println("==> Seed: nada a fazer.")
	fmt.Println("    Admin: migration 000007")
	fmt.Println("    Catálogo: go run ./cmd/import-catalog")
}
