# Decisões de arquitetura

## Contexto

Este desafio técnico foi recebido para uma vaga de **Backend Developer Júnior** (Go), remota, onde a descrição da vaga pede conhecimento inicial de Go/backend/SQL, e cita concorrência em Go e Uber Fx como **diferenciais**, não requisitos. O desafio em si, porém, cobra padrões de nível sênior (outbox transacional, idempotência distribuída, Keycloak/OIDC, SQS com DLQ, reconciliação financeira).

Diante desse descompasso, priorizei entregar **um núcleo funcional correto e bem testado**, cobrindo os fundamentos que o desafio testa de verdade (dinheiro exato, concorrência segura, idempotência, ledger auditável), e documentar explicitamente o que ficou fora de escopo em vez de tentar implementar tudo superficialmente.

## O que foi implementado

### Domínio (DDD, sem dependências de infraestrutura)

- `Money`: value object imutável, `int64` em centavos, parsing seguro (rejeita `NaN`, notação científica, escala excedente), aritmética com verificação de overflow.
- `Wallet`: agregado raiz com saldo, versão (lock otimista), `Debit`/`Credit` que nunca permitem saldo negativo.
- `WalletLedgerEntry`: registro imutável, valida `balanceAfter = balanceBefore ± amount` na construção.
- `WagerTransaction`: máquina de estados (`PENDING` → `PROCESSED`/`REJECTED`/`FAILED`/`PENDING_REFERENCE`), com regras de valor por tipo (BET/WIN/REFUND/ROLLBACK > 0, LOSS = 0) e separação entre transações externas e a `OPENING` interna.

### Infraestrutura

- Uber Fx cuidando da injeção de dependência e lifecycle (`OnStart`/`OnStop`) do servidor HTTP e do pool de conexões Postgres.
- PostgreSQL real via Docker Compose, com constraints de banco reforçando invariantes do domínio (`CHECK (balance_minor_units >= 0)`, `UNIQUE (wallet_id, transaction_id)`, `UNIQUE (provider_id, idempotency_key)`).
- Repositórios (`WalletRepository`, `LedgerRepository`, `WagerTransactionRepository`) implementando o lock otimista via `UPDATE ... WHERE id = ? AND version = ?`.

### Camada de aplicação e HTTP

- `WalletService`: orquestra abertura de carteira e processamento de transações, incluindo o fluxo de idempotência (verifica `(providerId, idempotencyKey)` antes de processar).
- Endpoints REST: `POST /wallets`, `GET /wallets/{id}`, `POST /wagering/transactions`.

### Testes

- Testes unitários dos 4 pacotes de domínio, incluindo o cenário obrigatório de concorrência (carteira com R$100 recebendo duas apostas de R$80 simultâneas), rodando com `go test -race`.
- Testes de integração dos repositórios contra Postgres real, incluindo o teste de versão obsoleta (`ErrStaleVersion`).
- Testes de integração HTTP de ponta a ponta (`internal/httpapi/integration_test.go`), incluindo **duas requisições HTTP concorrentes reais** batendo no mesmo Postgres — a versão mais realista possível do teste de concorrência do README dentro do escopo deste projeto.

## O que foi deixado de fora, e por quê

| Item                                                  | Por que foi cortado                                                                                                                                                                                                                                                                        | O que seria necessário                                                                                                                           |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Outbox transacional completo**                      | Exige coordenar commit da transação de negócio com publicação de evento de forma atômica — padrão avançado de sistemas distribuídos, fora do escopo real da vaga júnior.                                                                                                                   | Tabela `outbox`, worker de publicação, garantia de "publica só depois do commit".                                                                |
| **SQS com DLQ e redrive policy**                      | Não implementado. O foco foi validar a API REST síncrona, que é o que a vaga pede em primeiro lugar ("implementar endpoints REST").                                                                                                                                                        | Consumidor SQS, fila de erro, política de redrive, deduplicação na inbox.                                                                        |
| **Keycloak/OIDC completo**                            | Implementada apenas uma versão simplificada (token estático via header `Authorization: Bearer`, ver `internal/httpapi/auth_middleware.go`) em vez da integração OIDC real — suficiente para provar a necessidade de autenticação sem a complexidade de um provedor de identidade completo. | Integração OIDC, validação de token JWT, isolamento por `providerId` via claim do token.                                                         |
| **Retry com backoff / `PENDING_REFERENCE`**           | O estado `PENDING_REFERENCE` existe no domínio (`WagerTransaction.MarkPendingReference`), mas não há worker que resolve referências de forma assíncrona com backoff exponencial.                                                                                                           | Worker de retry, política de backoff, transição para `REJECTED`/`FAILED` após N tentativas.                                                      |
| **Validação completa de REFUND/ROLLBACK**             | `REFUND`/`ROLLBACK` creditam a carteira sem validar se a transação referenciada existe, está no estado correto, ou se já foi revertida antes.                                                                                                                                              | Busca da transação original por `externalTransactionId`, validação de estado e valor, bloqueio de reversão duplicada.                            |
| **Reconciliação financeira**                          | Não implementado — exigiria comparar o ledger interno com dados externos periodicamente.                                                                                                                                                                                                   | Job de reconciliação, relatório de divergências.                                                                                                 |
| **S3 e Secrets Manager (AWS)**                        | Não foram necessários no escopo atual — não há upload de arquivos, e o único segredo do projeto (senha do Postgres, token de auth) já é gerenciado via variável de ambiente (`.env`), suficiente para desenvolvimento local.                                                               | Em produção: Secrets Manager para credenciais de banco/token de auth, S3 para armazenamento de comprovantes ou logs de auditoria, se necessário. |
| **Observabilidade (métricas, tracing)**               | Fora de escopo dado o tempo disponível.                                                                                                                                                                                                                                                    | OpenTelemetry, métricas de latência/erro por endpoint.                                                                                           |
| **Concorrência entre múltiplas instâncias/processos** | Testado com múltiplas goroutines dentro de um único processo (via `httptest`), não com múltiplas instâncias do binário rodando em paralelo.                                                                                                                                                | Ambiente com load balancer + múltiplas réplicas da API.                                                                                          |

## Estratégia de concorrência

Optimistic locking via campo `version`, sem qualquer lock global:

1. Carrega a carteira (`FindByID`), guarda a versão atual.
2. Aplica a mutação em memória (`Wallet.Debit`/`Credit`), que já incrementa a versão.
3. Persiste com `UPDATE ... WHERE id = ? AND version = ?` (a versão antiga).
4. Se nenhuma linha for afetada, outra requisição já mudou a carteira primeiro → `ErrStaleVersion`.

Neste projeto, ao detectar `ErrStaleVersion`, a transação é marcada como `FAILED` com o código `CONCURRENT_UPDATE_CONFLICT` em vez de repetir automaticamente a operação. Uma implementação de produção retomaria do passo 1 (recarregar a carteira e tentar de novo, com limite de tentativas).
