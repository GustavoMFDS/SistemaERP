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
- A fila guarda somente o payload necessario para recriar a venda, o cabecalho `Idempotency-Key` e metadados mínimos de reconciliação (estado/erro/última tentativa).
- Evite incluir dados pessoais sensiveis no payload de venda offline. Quando o cliente for opcional, prefira venda sem identificacao.
- `localStorage` nao e um cofre criptografico. Nao ha chave segura no frontend para criptografia forte sem apoio do usuario/dispositivo.
- A funcao `clearOfflineQueue()` permite limpeza manual controlada quando o operador precisar descartar pendencias locais.
- Caixa, cache de produtos e fila usam chaves derivadas de `tenant_id + user_id`, impedindo que outro tenant/usuário leia o estado anterior no mesmo navegador. Logout limpa apenas o escopo atual antes de remover o access token.
- A fila legada global `sistemaemgo:offlineQueue:v1` nunca é executada automaticamente. Se detectada após upgrade, o operador pode importá-la explicitamente para revisão; os itens entram em `attention` e exigem retry manual.
- Ao vincular um item de atenção a um caixa atual, a `Idempotency-Key` original é preservada. Se a operação original já tiver sido commitada com payload diferente, o backend responde conflito em vez de aceitar uma segunda venda sob uma chave nova.
- Cabecalhos sensiveis como `Authorization`, cookies e tokens nao sao persistidos na fila.
- O backend usa `request_hash`: mesma chave + mesmo hash reaproveita o resultado; mesma chave + hash diferente retorna `409 conflict`.



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
- falha de rede durante logout não limpa estado local nem simula revogação do cookie HttpOnly.

## Ciclo de caixa

- O PDV fecha o caixa chamando `POST /api/v1/cash/sessions/{id}/close`; remover apenas a referência local não encerra uma sessão.
- A migration `0013_single_open_cash_session` cria um índice único parcial para permitir somente uma sessão `open` por tenant/registro.
- Se a migration encontrar duplicatas já abertas, ela aborta e exige reconciliação operacional; não fecha sessões automaticamente.
- A migration `0014_cash_reconciliation` persiste `expected_cash` e `closing_difference`. No fechamento, o backend calcula abertura + pagamentos em dinheiro de vendas finalizadas, compara com o valor declarado e grava/audita a diferença.


## Leitura de codigo de barras

- Produtos possuem `barcode` opcional e a unicidade e por tenant/loja, permitindo o mesmo EAN/GTIN em empresas independentes.
- No PDV, leitores USB/Bluetooth que operam como teclado podem preencher o campo de codigo e enviar `Enter`.
- O primeiro scan adiciona o produto ao carrinho; scans seguintes do mesmo produto incrementam a quantidade em vez de criar linhas duplicadas.
- O PDV tenta resolver primeiro pelo catalogo ja carregado no navegador. Se o codigo nao estiver no cache e houver conexao, consulta `GET /api/v1/products/barcode/{barcode}`.
- Offline, um codigo so pode ser resolvido se o produto estiver no cache local previamente carregado. O sistema nao inventa nem aceita produto desconhecido durante a queda de rede.


## Carrinhos suspensos

Carrinho suspenso e fila offline são conceitos diferentes:

- carrinho suspenso é um rascunho local ainda não finalizado e não possui `Idempotency-Key`;
- fila offline representa uma intenção exata de venda já finalizada pelo operador e persistida por write-ahead;
- carrinhos suspensos são escopados por tenant+usuário no navegador;
- retomar um carrinho não envia nenhuma requisição; o write-ahead só ocorre quando o operador finaliza;
- descontos continuam sujeitos à permissão server-side `sale:discount` quando a venda é enviada ou reexecutada.


## Validade do catálogo offline

- O cache de produtos é escopado por tenant+usuário e guarda `savedAt` junto dos itens.
- O PDV aceita fallback de catálogo por no máximo 24 horas desde a última atualização online bem-sucedida.
- Cache legado sem timestamp é tratado como não confiável para novas vendas até que haja uma atualização online.
- Cache expirado ou inválido não é usado para formar carrinho; o operador precisa reconectar e atualizar o catálogo.
- Carrinhos suspensos retomados online recarregam o catálogo e reaplicam o preço efetivo atual antes de voltar ao carrinho ativo.
- O preço efetivo do PDV segue a mesma regra do backend: `promo_price` positivo quando presente, caso contrário `price_cash`.


## Carrinhos suspensos e logout

- Carrinho suspenso é apenas rascunho local, escopado por tenant+usuário; não cria venda nem reserva estoque.
- O navegador mantém no máximo 20 carrinhos suspensos e rejeita o próximo em vez de descartar silenciosamente um rascunho antigo.
- Se houver fila offline pendente ou carrinho suspenso, o logout pede confirmação explícita antes de limpar os dados locais.
- Falha ao revogar a sessão no servidor preserva token, fila, cache e carrinhos suspensos.
- Carrinhos suspensos também são removidos no logout confirmado, adequado para terminais compartilhados de loja.
