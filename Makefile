DB_URL ?= pgx5://blogdog:blogdog@localhost:5432/blogdog?sslmode=disable
MIGRATE := $(HOME)/go/bin/migrate
MIGRATE_ARGS := -database "$(DB_URL)" -path ./migrations

.PHONY: db-reset db-migrate db-rollback

db-reset: ## DBを初期化（全データ削除して再作成）
	$(MIGRATE) $(MIGRATE_ARGS) down -all
	$(MIGRATE) $(MIGRATE_ARGS) up

db-migrate: ## マイグレーションを適用
	$(MIGRATE) $(MIGRATE_ARGS) up

db-rollback: ## マイグレーションをすべてロールバック
	$(MIGRATE) $(MIGRATE_ARGS) down -all
