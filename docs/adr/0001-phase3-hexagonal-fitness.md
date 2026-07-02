# ADR 0001 — Phase 3: Hexagonal (ports & adapters) + Fitness Functions

Status: Proposed (Phase 3, выполнять ПОСЛЕ Phase 2: полный v2 envelope + уведомления)
Inspiration: архитектура коллеги — «Fitness Functions in Software Architecture: measuring things to ensure prosperity». Гексагон, чьи границы закреплены как код (guard-тесты в `make lint`, валящие сборку при нарушении слоёв).

## Зачем
Сейчас emplacc-main-api — «прото-гексагон»: слои controller→service→repository, у сервисов и репозиториев есть интерфейсы, DI в `cmd/server/main.go`, ctx-гигиена уже чистая. Но границы НЕ закреплены (ничто не мешает им деградировать), интерфейсы лежат рядом с реализациями, нет композиционного корня как слоя, есть god-файл `domain/models.go`, и часть драйверов протекает в слой сервисов.

## Целевая структура (адаптировано под стек: Go 1.26, Echo, GORM/Postgres, Redis, MinIO/rustfs, gRPC→reports_llm_ms, Keycloak)
```
cmd/{server,doctor,openapi-gen}
internal/
  app/                         ← КОМПОЗИЦИОННЫЙ КОРЕНЬ: Bootstrap + containers_*.go.
                                  Единственное место, что «знает про всё»: создаёт адаптеры,
                                  прячет за порты, внедряет в сервисы, сервисы — в хендлеры.
  transport/http/              ← вход (из controller): handlers → presenter → dto, middleware, validation.
                                  Хендлеры импортируют ТОЛЬКО ports/services, НИКОГДА repository/драйверы.
  services/                    ← домен/бизнес-логика (из service). Запрещён импорт transport,
                                  сырых драйверов (gorm, go-redis, minio-go, gocloak) и net/http.
  models/                      ← доменные сущности, РАЗБИТЫ по доменам (ломаем models.go: user.go,
                                  task.go, project.go, board.go, ... ; conveyor.go уже отдельно).
  ports/                       ← чистые интерфейсы-контракты, ОДИН домен на файл (god-файлы
                                  service.go/repository.go запрещены, лимит строк). Это граница гексагона.
  repository/{postgres,redis,minio,fs}  ← выходные адаптеры, реализуют ports.
  infra/                       ← keycloak-adapter, llm-grpc-adapter, mail, health-пробы.
  architecture/guards_test.go  ← ФИТНЕС-ФУНКЦИИ (см. ниже).
  observability/ resilience/ config/
```

## Фитнес-функции (internal/architecture/guards_test.go, гоняются в `make lint`)
- **R1 (transport чистый):** `internal/transport/**` и middleware не импортируют `internal/repository/**` и сырые драйверы.
- **R2 (домен чистый):** `internal/services/**` не импортируют transport, `gorm.io/gorm`, `go-redis`, `minio-go`, `gocloak`, `net/http`.
- **R3 (ctx-гигиена):** запрещены `if ctx == nil` и переприсваивание `ctx = context.Background()` по `internal/`+`api/`. (Уже 0 нарушений — держим green.)
- **R4 (DTO-контракт):** json-теги в dto только snake_case (FE↔BE).
- **R5 (порты):** один домен на файл в `internal/ports`, нет god service.go/repository.go, лимит строк на файл.
- **R6 (пагинация):** List-методы репозиториев нормализуют пагинацию (safe ListOptions).
Реализация: проход по AST/импортам пакетов (`go/parser`, `golang.org/x/tools/go/packages`) + allowlist текущего легаси-долга.

## Gap (что чинить — найдено сверкой на 2026-06-21)
- `internal/controller/role_middleware.go` импортирует `repository` (RequireRoles берёт RoleRepository) → нарушение R1.
- `internal/service/Auth.go` использует `net/http` (token-exchange) + gocloak → R2.
- `internal/service/Storage.go` использует minio-go в слое сервисов → R2 (вынести за порт StoragePort + adapter).
- `internal/domain/models.go` — god-файл (369 строк) → разбить (R5-аналог для моделей).
- Нет `internal/ports`, `internal/app`, `internal/architecture`, `make lint`.
- Хорошо: ctx-гигиена чистая (R3 = 0), conveyor.go уже отделён, сервисы/репозитории уже за интерфейсами.

## План миграции (guards-first, как у коллеги: зафиксировать → ремедиация фазами → «complete remediation»)
- **3.0** Добавить `guards_test.go` + `make lint` с allowlist ТЕКУЩИХ нарушений (role_middleware→repo, Auth net/http, Storage minio, god models.go). Сборка зелёная, НОВЫЕ нарушения блокируются.
- **3.1** Вынести интерфейсы в `internal/ports` (один домен на файл).
- **3.2** Разбить `domain/models.go` → `internal/models/*` по доменам.
- **3.3** Композиционный корень `internal/app` (перенести проводку из main.go в Bootstrap + containers).
- **3.4** repository → выходные адаптеры (`repository/postgres` и т.д.); Storage(minio) и Keycloak/net-http за порты в `infra/`; сократить allowlist до пустого.
- **3.5** controller → `transport/http` (handlers→presenter→dto); убрать импорт repo из middleware.
- На каждом шаге guards остаются зелёными; allowlist сжимается по мере выплаты долга.

## Замечания
- Делать ПОСЛЕ Phase 2 (иначе v2/уведомления пришлось бы переносить дважды).
- Фронт emplacc-web уже генерит типы из swagger; codegen TS-типов (как `dto-tsgen` у коллеги) — опционально позже.
- SSE `EventHub`/`Stream` (Phase 1) встанут как порт `EventBusPort` + adapter (in-memory → Redis pub/sub).
