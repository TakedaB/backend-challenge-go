# Backend Challenge — Jungle Gaming (Wager Ledger)

API em Go para abertura de carteiras e processamento de operações de apostas (BET, WIN, LOSS, REFUND, ROLLBACK), com persistência em PostgreSQL, injeção de dependência via Uber Fx, idempotência e controle de concorrência via lock otimista.

> **Nota de escopo:** este projeto implementa o núcleo financeiro do desafio original (dinheiro exato, concorrência segura, idempotência, ledger auditável), mas corta deliberadamente partes do desafio que exigem experiência de nível sênior (outbox transacional completo, SQS com DLQ, Keycloak/OIDC, retry com backoff, reconciliação). Essas decisões estão detalhadas em [`ARCHITECTURE.md`](./ARCHITECTURE.md).

## Stack

- Go 1.22+
- Uber Fx (injeção de dependência e lifecycle)
- PostgreSQL 16 (via `pgx/v5`)
- Docker Compose

## Como rodar

### 1. Pré-requisitos

- Go 1.22 ou superior
- Docker e Docker Compose

### 2. Configuração

Copie o arquivo de exemplo de variáveis de ambiente:

```bash
cp .env.example .env
```

### 3. Suba o Postgres

```bash
docker compose up -d
```

### 4. Aplique as migrations

```bash
Get-Content .\migrations\001_create_wallets.sql | docker exec -i backend-challenge-go-postgres psql -U app -d backend_challenge
Get-Content .\migrations\002_create_wallet_ledger_entries.sql | docker exec -i backend-challenge-go-postgres psql -U app -d backend_challenge
Get-Content .\migrations\003_create_wager_transactions.sql | docker exec -i backend-challenge-go-postgres psql -U app -d backend_challenge
```

### 5. Rode a API

```bash
go run ./cmd/api
```

O servidor sobe em `http://localhost:8080`.

### 6. Rode os testes

```bash
go test -race -count=1 ./...
```

Os testes em `internal/infra/postgres` e `internal/httpapi` são de integração e exigem o Postgres do passo 3 rodando.

## Autenticação

Todos os endpoints exceto `/health` exigem o header:

Authorization: Bearer <API_AUTH_TOKEN>

O valor padrão em desenvolvimento (`.env.example`) é `dev-secret-token`.

> Esta é uma versão simplificada de autenticação (token estático), usada no lugar de uma integração OIDC/Keycloak completa. Ver [`ARCHITECTURE.md`](./ARCHITECTURE.md) para a justificativa dessa decisão de escopo.

## Endpoints

### `POST /wallets` — abre uma carteira

```json
{
  "playerId": "player-1",
  "currency": "BRL",
  "initialBalance": "100.00"
}
```

Resposta `201 Created`:

```json
{
  "id": "...",
  "playerId": "player-1",
  "currency": "BRL",
  "balance": "100.00",
  "version": 1,
  "updatedAt": "..."
}
```

### `GET /wallets/{id}` — consulta saldo

Resposta `200 OK` com o mesmo formato acima. `404` se a carteira não existir.

### `POST /wagering/transactions` — processa uma operação

```json
{
  "externalTransactionId": "ext-001",
  "providerId": "provider-a",
  "idempotencyKey": "idem-001",
  "walletId": "...",
  "playerId": "player-1",
  "roundId": "round-1",
  "gameId": "game-1",
  "kind": "BET",
  "amount": "30.00",
  "currency": "BRL"
}
```

`kind` aceita: `BET`, `WIN`, `LOSS`, `REFUND`, `ROLLBACK`. `REFUND` e `ROLLBACK` exigem `referenceExternalTransactionId`. `LOSS` exige `amount: "0.00"`.

Resposta `200 OK` (processado ou rejeitado) ou `422 Unprocessable Entity` (rejeitado por regra de negócio):

```json
{
  "id": "...",
  "externalTransactionId": "ext-001",
  "walletId": "...",
  "kind": "BET",
  "amount": "30.00",
  "state": "PROCESSED"
}
```

Requisições repetidas com o mesmo `(providerId, idempotencyKey)` retornam a transação original sem reprocessar. A chave pode ser enviada no header `Idempotency-Key` ou no campo `idempotencyKey` do corpo — se o header estiver presente, ele tem prioridade sobre o campo do body.

## Estrutura do projeto

cmd/api — entrypoint (Fx wiring)
internal/domain/money — value object de dinheiro (int64, sem float)
internal/domain/wallet — agregado Wallet (saldo, versão, Debit/Credit)
internal/domain/ledger — WalletLedgerEntry (registro imutável)
internal/domain/wagertransaction — máquina de estados das operações
internal/app — casos de uso (orquestra domínio + repositórios)
internal/httpapi — handlers HTTP, DTOs, roteamento, auth
internal/infra/postgres — repositórios e conexão com Postgres
internal/support/idgen — geração de UUID
migrations — SQL de criação das tabelas

## Decisões técnicas principais

- **Dinheiro em `int64` (centavos), nunca `float`** — evita erro de arredondamento em cálculo financeiro.
- **Concorrência via lock otimista (campo `version`)** — sem locks globais; carteiras diferentes processam em paralelo. Testado com requisições HTTP simultâneas reais (ver `internal/httpapi/integration_test.go`).
- **Ledger append-only** — nunca há `UPDATE`/`DELETE` em `wallet_ledger_entries`; correções seriam novos lançamentos.
- **Idempotência via `(providerId, idempotencyKey)`** — índice único no banco.
- **Autenticação simplificada (bearer token estático)** — ver seção [Autenticação](#autenticação) acima.

Detalhes completos de arquitetura e o que foi conscientemente deixado de fora: [`ARCHITECTURE.md`](./ARCHITECTURE.md).
