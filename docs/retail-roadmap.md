# Roadmap de varejo

Ordem acordada para evoluir o SistemaEmGo para uso nas lojas da familia. Cada loja/empresa continua sendo um tenant independente.

## 2. Codigo de barras e scanner — implementado no PR integrado #15; aguardando CI/merge

Criterios de aceite:

- barcode opcional no cadastro de produto;
- mesmo EAN/GTIN permitido em tenants diferentes, mas unico dentro do mesmo tenant;
- busca exata por barcode no backend;
- leitor USB/Bluetooth em modo teclado funciona no PDV;
- Enter adiciona o produto e scans repetidos incrementam quantidade;
- fallback online consulta o backend quando o produto nao estiver no cache local;
- offline nunca aceita codigo desconhecido;
- testes de integracao e E2E cobrem isolamento e scanner.

## 3. Fornecedores, compras e entrada de estoque — implementado no PR integrado #15; aguardando CI/merge

- cadastro de fornecedores;
- pedido/compra com itens, custo e status;
- recebimento parcial ou total;
- entrada de estoque vinculada a compra;
- historico de custo;
- contas a pagar/ledger quando aplicavel;
- auditoria e isolamento por tenant.

## 4. Trocas e devolucoes — implementado no PR integrado #15; aguardando CI/merge

- devolucao total/parcial de venda;
- motivo e operador responsavel;
- retorno de estoque quando aplicavel;
- estorno/credito de pagamento desacoplado da devolucao fisica;
- trilha de auditoria e protecao contra dupla devolucao.

## 5. Pagamentos e conciliacao — implementado no PR integrado #15; aguardando CI/merge

- detalhamento de Pix, debito, credito e dinheiro;
- identificadores de transacao/adquirente quando disponiveis;
- conciliacao por periodo e metodo;
- divergencias e ajustes auditados;
- preparar integracao TEF/adquirentes sem acoplar o dominio a um unico provedor.

## 6. Melhorias operacionais do PDV — implementado no PR integrado #15; aguardando CI/merge

- atalhos de teclado;
- busca rapida;
- foco automatico no scanner;
- alteracao de quantidade e remocao eficiente;
- desconto com permissao;
- suspender/retomar venda, se necessario;
- feedback claro de estoque, caixa, offline e erros;
- impressao/recibo nao fiscal enquanto o fiscal real estiver pendente.

## 7. Piloto real — preparação implementada; execução externa pendente

- ambiente production-like dedicado a uma loja;
- backup/restore comprovado;
- monitoramento e alertas;
- usuarios/permissoes reais;
- carga inicial de produtos/estoque;
- operacao paralela controlada com o processo atual;
- checklist de abertura, venda, fechamento, devolucao e reconciliacao;
- coleta de problemas operacionais antes da expansao.

## 1. NFC-e / SEFAZ — por ultimo

A integracao fiscal real fica para a etapa final, depois que o fluxo comercial e operacional estiver estabilizado. O provider MVP permanece proibido em staging/producao.


### Etapa 4 — decisões implementadas

- devolução parcial ou total vinculada aos itens da venda original;
- quantidade acumulada nunca pode exceder a quantidade vendida;
- valor devido é calculado pelo backend a partir do valor líquido original;
- `restock=true` devolve estoque vendável; `restock=false` preserva a baixa para item avariado/não revendável;
- troca é uma devolução do tipo `exchange` seguida por uma nova venda normal no PDV;
- reembolso financeiro permanece pendente para a etapa 5;
- uma venda com qualquer devolução registrada não pode mais ser cancelada integralmente.


### Etapa 5 — decisões implementadas

- pagamentos aceitam metadados opcionais de provedor, transação, autorização e parcelas sem acoplamento a uma adquirente específica;
- dinheiro continua conciliado no fechamento físico do caixa;
- PIX, débito, crédito, transferência e voucher podem ser conciliados por valor recebido, taxa, provedor e referência externa;
- divergências ficam explícitas e a conciliação inicial permanece imutável;
- correções posteriores usam ajustes separados, idempotentes e auditados, preservando valores/taxas anteriores e novos;
- reembolsos de devoluções podem ser liquidados parcialmente em um ou mais métodos;
- a soma liquidada nunca pode exceder o `refund_due`;
- reembolso em dinheiro exige caixa aberto, saldo físico suficiente e gera `withdrawal`;
- reembolsos digitais associados a uma sessão reduzem o esperado daquele método no fechamento;
- toda liquidação gera lançamento negativo `return_refund` no ledger;
- TEF/adquirentes futuros podem preencher os mesmos campos sem alterar o domínio de venda.


### Etapa 6 — decisões implementadas

- F2 move o foco para o scanner, F4 para a busca rápida e F8 finaliza a venda;
- busca rápida filtra por nome, SKU ou código de barras;
- quantidade pode ser alterada inline com botões +/- ou entrada direta;
- carrinhos podem ser suspensos e retomados localmente, sempre escopados por tenant+usuário;
- carrinho suspenso não é tratado como venda nem enviado ao servidor até a finalização;
- desconto de venda é exibido apenas para quem possui `sale:discount`;
- o backend também bloqueia qualquer desconto sem `sale:discount`, inclusive chamadas API manipuladas;
- após venda online, o catálogo/estoque é atualizado e o foco retorna ao scanner;
- vendas finalizadas exibem opção de impressão de comprovante explicitamente não fiscal.


### Etapa 7 — preparação implementada

- runbook de piloto controlado em uma loja;
- template versionado de evidências e decisão go/no-go;
- checker read-only de prontidão para ambiente production-like;
- checker valida health/live, health/ready, migration limpa >= 22, objetos críticos, ausência de usuários demo, sessões de caixa duplicadas e papéis tenant-scoped;
- CI valida a sintaxe do checker;
- backup/restore continua seguindo `docs/deployment.md` e só pode ser marcado como testado com evidência do ambiente real;
- a execução do piloto, reconciliação com operação física, monitoramento real e aprovações de owner/accounting permanecem pendências externas;
- fiscal continua fora do piloto e permanece bloqueado até implementação SEFAZ-ready.
