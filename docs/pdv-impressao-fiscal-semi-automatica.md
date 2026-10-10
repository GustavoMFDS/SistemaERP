# PDV: emissão fiscal obrigatória e impressão térmica opcional

## Regra aprovada (09/10/2026)

A pergunta ao cliente é somente **sobre imprimir papel**. Não é pergunta
sobre emitir nota: toda venda confirmada tem obrigação fiscal, mesmo quando
o cliente não deseja uma via física.

### Fluxo do operador

1. Finalizar pagamento/venda com o ID de idempotência original.
2. O PostgreSQL confirma o lançamento e, na **mesma transação**, insere
   `sale_fiscal_intents` (migration 0039). O cliente não tem opção de
   desativá-la. O modelo padrão é NFC-e (65), para varejo presencial;
   operações que exijam NF-e (55) precisarão do fluxo adequado de
   identificação do destinatário e das regras fiscais pertinentes.
3. O PDV consulta `GET /api/v1/sales/{id}/fiscal-status`, autorizada por
   `sale:read` ou `sale:write`, sempre filtrada pelo tenant do operador.
4. Após uma venda online confirmada, perguntar:
   **"O cliente deseja imprimir o DANFE NFC-e?"**
   - **Não imprimir** apenas fecha essa pergunta; não cancela venda nem
     pendência fiscal. O documento autorizado continua consultável pela
     administração. A chave de acesso pode ser copiada quando autorizada.
   - **Sim, abrir para imprimir** só fica disponível quando a NFC-e tem
     `status=authorized`, modelo 65, protocolo, data de autorização e chave
     de acesso. Abre o DANFE pelo backend, seguido do diálogo de impressão
     do navegador. Não envia comandos ESC/POS diretamente ao dispositivo.
5. Se a emissão não está pronta, mostrar **pendente**, sem dizer que o
   documento foi emitido. O operador pode atualizar o status.
6. Vendas armazenadas offline ainda não são vendas confirmadas no servidor:
   não há nota autorizada nem janela de impressão até a sincronização
   segura e confirmação na API. O gatilho registra a pendência fiscal quando
   a venda finalmente for confirmada no servidor.

### Segurança e limites

- `0039`: backfill de vendas anteriores com `legacy_review=true` para
  conciliação, sem atribuir autorização fiscal retroativamente; trigger
  pós-INSERT cria obrigações para todas as novas vendas, mesmo que outra
  integração insira registros diretamente. A relação usa
  `(tenant_id, sale_id)` e impede ligação entre CNPJs.
- Uma NFC-e rejeitada, reservada, assinada ou submetida **não é autorizada**.
  Impressão normal é bloqueada até o status/protocolo real; o renderer do
  DANFE também aplica seus próprios controles.
- A URL para impressão do PDV revalida autenticação e o tenant, exige
  `sale:read` ou `sale:write`, aplica rate limit e registra auditoria;
  não expõe XML nem acesso à configuração fiscal ao caixa.
- O botão usa o diálogo do sistema operacional. O driver, spooler,
  largura de papel (o template atual usa 80 mm) e modelo físico
  precisam de testes na impressora da loja. A PWA não precisa ser nativa.
- Para atender à dispensa de papel quando permitida pela UF, é preciso
  disponibilizar a NFC-e/chave ou alternativa eletrônica válida conforme
  a legislação da operação. A cópia da chave não implementa por si só
  envio por e-mail, SMS ou WhatsApp.

### Limite de implantação — NÃO considerar emissão automática pronta

O repositório já dispõe das fundações de **NFC-e**, mas a criação automática
da `sale_fiscal_intents` **não reserva número, não calcula tributos, não
assina XML e não autoriza na SEFAZ**. A NF-e (modelo 55) ainda não tem
pipeline completo de autorização de produção. A política de registro de
pendências foi implementada, não a emissão completa.

Antes de permitir uma loja fiscalmente operacional, implementar e provar
um orquestrador durável/idempotente para cada modelo:
- validação do emitente/CNPJ, certificados e perfil fiscal por SKU;
- aplicação correta das regras tributárias por UF/regime/cliente, além
  de seleção NFC-e versus NF-e;
- reserva da numeração, geração, assinatura, transmissão e autorização,
  com reconciliação após timeout/resposta ambígua;
- rejeição, contingência legal, eventual cancelamento, reemissão
  controlada e acompanhamento centralizado de pendências;
- habilitação somente após homologação real, com evidência por empresa;
- testes em impressoras térmicas 58/80 mm reais e em falhas de rede,
  entrega digital e reimpressão posterior.

Em especial, **não concluir a venda como fiscalmente emitida** quando
somente o registro obrigatório está salvo. Até que as etapas acima estejam
homologadas, o sistema deve permanecer em piloto sem emissão fiscal real.

### Evidências locais

- Banco isolado: migração 0039 clean; 23 vendas antigas classificadas
  como legado para revisão, sem vendas fora de `sale_fiscal_intents`.
- `TestProductVariationSaleReturnIsolatesEveryBalance`: venda real em
  PostgreSQL com Redis, registro fiscal obrigatório e idempotência
  confirmados 2x sem SEFAZ.
- `pdv-fiscal-print-choice.spec.ts`: escolha "Não imprimir" preserva a
  venda; sem autorização, botão "Sim" permanece indisponível e a
  interface não tenta baixar DANFE (API simulada).
- `pdv-operations.spec.ts` atualizado para a regra nova no fluxo
  prod-like com API real: requer execução na próxima rodada E2E completa.
