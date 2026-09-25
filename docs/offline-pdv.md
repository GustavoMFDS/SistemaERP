# Offline PDV (Etapa 13)

Objetivo: permitir que o PDV continue operando em quedas de rede **curtas/intermitentes**, registrando vendas localmente e sincronizando depois, sem duplicar lançamentos.

## Escopo (MVP)

- Persistência local: fila de requisições no `localStorage` (browser), isolada por tenant + usuário
- Fila de eventos: cada venda (`POST /api/v1/sales`) é registrada como um item na fila
- Sincronização posterior: ao voltar online, o frontend drena a fila em ordem
- Segurança/consistência: idempotência no backend para retries (`Idempotency-Key`)

## Frontend

- Página: `web/src/pages/PDVPage.tsx`
- Fila: `web/src/lib/offlineQueue.ts`

Fluxo:
1. PDV monta e tenta sincronizar pendências se `navigator.onLine`.
2. Em `Finalizar`, uma trava síncrona permite apenas uma intenção em voo; uma única `Idempotency-Key` é gerada.
3. **Write-ahead:** antes do primeiro `POST`, a intenção completa é persistida na fila local com essa chave. Se o browser não conseguir persistir a intenção (por exemplo, quota/storage indisponível), nenhum request de venda é enviado.
4. Se o browser estiver offline, a venda já permanece pendente. Se a resposta se perder após o envio, a mesma intenção/chave já persistida é reutilizada no retry.
5. Evento `online` dispara `flushQueue()` para sincronizar.
6. Rejeições permanentes 4xx ficam preservadas como itens que requerem atenção e não bloqueiam vendas posteriores; falhas transitórias/rede interrompem o flush para retry posterior.

Observação: o `cash_session_id` precisa já existir (caixa aberto previamente). Este MVP não tenta “abrir caixa offline”.

## Backend

- Header obrigatório: `Idempotency-Key`
- Rota: `POST /api/v1/sales`
- Implementação:
  - `pg_advisory_xact_lock` serializa concorrência por chave
  - tabela `idempotency_keys` guarda o resultado (sale_id + total)

Migração:
- `backend/migrations/0004_idempotency_keys.up.sql`

## Como testar

1. Abrir o PDV online e carregar produtos.
2. Simular offline no browser (DevTools → Network → Offline).
3. Finalizar uma venda: deve aparecer como “registrada offline (pendente sync)”.
4. Voltar online: o PDV sincroniza automaticamente.

## Seguranca e privacidade operacional

- Após 24 horas, itens ainda não sincronizados deixam de ser reenviados automaticamente e passam para estado de atenção; o registro local é preservado para reconciliação.
- O backend retém resultados idempotentes por 30 dias. Para manter margem contra clock skew/maintenance, retry/rebind manual no browser só é permitido até 28 dias desde a criação original da intenção. Itens mais antigos permanecem visíveis em `attention`, mas exigem conferência manual no servidor e não são reenviados automaticamente.
- Em upgrade de filas v2 anteriores que já tiveram retry/rebind, a versão antiga pode ter sobrescrito `createdAt`; nesses casos a idade original é considerada não confiável. O item entra em `attention` com motivo `retention_unknown` e fica bloqueado para retry/rebind até conferência manual.
- A fila guarda somente o payload necessario para recriar a venda, o cabecalho `Idempotency-Key` e metadados mínimos de reconciliação (estado/erro/última tentativa).
- Evite incluir dados pessoais sensiveis no payload de venda offline. Quando o cliente for opcional, prefira venda sem identificacao.
- `localStorage` nao e um cofre criptografico. Nao ha chave segura no frontend para criptografia forte sem apoio do usuario/dispositivo.
- A funcao `clearOfflineQueue()` permite limpeza manual controlada quando o operador precisar descartar pendencias locais.
- Caixa, cache de produtos e fila usam chaves derivadas de `tenant_id + user_id`, impedindo que outro tenant/usuário leia o estado anterior no mesmo navegador. Trocar de CNPJ preserva cada namespace isolado; logout limpa os namespaces de todas as lojas do usuário antes de remover o access token.
- A fila legada global `sistemaemgo:offlineQueue:v1` nunca é executada automaticamente. Se detectada após upgrade, o operador pode importá-la explicitamente para revisão; os itens entram em `attention` e exigem retry manual.
- Como a fila v1 não possui ownership confiável de tenant/usuário, o logout não a apaga automaticamente. Enquanto houver itens legados, o logout pela UI é bloqueado até importação para revisão ou descarte explícito no PDV.
- Ao vincular um item de atenção a um caixa atual, a `Idempotency-Key` original é preservada. Se a operação original já tiver sido commitada com payload diferente, o backend responde conflito em vez de aceitar uma segunda venda sob uma chave nova.
- Cabecalhos sensiveis como `Authorization`, cookies e tokens nao sao persistidos na fila.
- O backend usa `request_hash`: mesma chave + mesmo hash reaproveita o resultado; mesma chave + hash diferente retorna `409 conflict`.
- Novas intenções de venda preservam `unit_price` como snapshot informativo do preço exibido ao operador. O backend nunca usa esse valor como autoridade: ele recalcula com o preço atual do produto e retorna `409 price_changed` se o snapshot divergir.
- Após `price_changed`, o PDV mantém a intenção em `attention`. O operador pode escolher **Atualizar preços**, que busca os preços atuais, recalcula o pagamento único e reenvia a intenção com a mesma `Idempotency-Key`; essa ação só é habilitada para o conflito explícito de preço.
- Filas v2 antigas que não possuem snapshot de preço entram em `attention` com `price_snapshot_missing` e não podem retry/rebind automaticamente. O sistema não inventa qual valor foi originalmente cobrado.



## Cenarios E2E obrigatorios

- duplo clique/submissão concorrente em Finalizar gera apenas uma intenção/requisição de venda;
- falha de `localStorage` antes do write-ahead bloqueia o envio: nenhum `POST /sales` pode sair sem a intenção/chave persistida;
- chamada direta de `POST /sales` sem `Idempotency-Key` deve ser rejeitada;
- venda offline normal e sincronizacao ao reconectar;
- resposta perdida depois de o backend processar a venda: retry deve usar a mesma `Idempotency-Key` e nao duplicar a venda;
- duplo clique em `Finalizar`: somente um POST de venda e uma chave idempotente devem ser emitidos;
- rebind de venda legada/attention para outro caixa: preservar a chave original e bloquear duplicação se o backend já tiver commitado a intenção;
- dois tenants/usuarios no mesmo browser nao compartilham caixa, cache de produtos ou fila;
- rejeicao permanente de um item nao impede a sincronizacao dos itens posteriores;
- item com mais de 24 horas permanece armazenado em estado de atencao, sem exclusao silenciosa;
- rebind de venda legada já commitada preserva a chave original e não duplica a venda;
- item com mais de 28 dias não pode ser retry/rebindado; permanece em `attention` com motivo `retention_expired` e nenhum `POST /sales` é emitido;
- item v2 pré-upgrade que já possua `lastAttemptAt` mas não `intentCreatedAt` é tratado como idade original desconhecida (`retention_unknown`) e também não pode emitir `POST /sales`;
- falha de rede durante logout não limpa estado local nem simula revogação do cookie HttpOnly;
- quantidade fracionada usa milésimos e arredondamento monetário por linha equivalente ao backend;
- mudança de preço entre criação da intenção e sync coloca a venda em `price_changed`, sem criar venda; após revisão explícita, a intenção pode ser recalculada e enviada uma única vez;
- item v2 pré-snapshot de preço entra em `price_snapshot_missing` e não emite `POST /sales`.

## Ciclo de caixa

- `GET /api/v1/cash/sessions/current` recupera a sessão `open` do caixa padrão do tenant sem criar registros durante a leitura.
- Ao abrir o PDV online, o frontend reconcilia o `cash_session_id` local com o servidor: recupera o ID perdido quando existe sessão aberta e remove referência local stale quando o servidor não possui sessão aberta.
- A abertura fica temporariamente bloqueada enquanto essa reconciliação inicial está em andamento, evitando race entre `GET /current` e `POST /open`.
- Ao voltar do offline para online, a reconciliação do caixa é executada novamente.
- Logout é bloqueado enquanto existirem IDs locais de caixa aberto em qualquer CNPJ do usuário; além disso, o frontend consulta `GET /api/v1/cash/sessions/open-by-me` para detectar caixas abertos pelo próprio usuário mesmo quando o ID local foi perdido. Se a consulta falhar, o logout é cancelado por segurança.

- O PDV fecha o caixa chamando `POST /api/v1/cash/sessions/{id}/close`; remover apenas a referência local não encerra uma sessão.
- A migration `0013_single_open_cash_session` cria um índice único parcial para permitir somente uma sessão `open` por tenant/registro.
- Se a migration encontrar duplicatas já abertas, ela aborta e exige reconciliação operacional; não fecha sessões automaticamente.
- A migration `0014_cash_reconciliation` persiste `expected_cash` e `closing_difference`. No fechamento, o backend calcula abertura + pagamentos em dinheiro de vendas finalizadas, compara com o valor declarado e grava/audita a diferença.
