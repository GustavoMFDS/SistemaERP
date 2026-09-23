# Roadmap de varejo

Ordem acordada para evoluir o SistemaEmGo para uso nas lojas da familia. Cada loja/empresa continua sendo um tenant independente.

## 2. Codigo de barras e scanner — em implementacao

Criterios de aceite:

- barcode opcional no cadastro de produto;
- mesmo EAN/GTIN permitido em tenants diferentes, mas unico dentro do mesmo tenant;
- busca exata por barcode no backend;
- leitor USB/Bluetooth em modo teclado funciona no PDV;
- Enter adiciona o produto e scans repetidos incrementam quantidade;
- fallback online consulta o backend quando o produto nao estiver no cache local;
- offline nunca aceita codigo desconhecido;
- testes de integracao e E2E cobrem isolamento e scanner.

## 3. Fornecedores, compras e entrada de estoque

- cadastro de fornecedores;
- pedido/compra com itens, custo e status;
- recebimento parcial ou total;
- entrada de estoque vinculada a compra;
- historico de custo;
- contas a pagar/ledger quando aplicavel;
- auditoria e isolamento por tenant.

## 4. Trocas e devolucoes

- devolucao total/parcial de venda;
- motivo e operador responsavel;
- retorno de estoque quando aplicavel;
- estorno/credito de pagamento desacoplado da devolucao fisica;
- trilha de auditoria e protecao contra dupla devolucao.

## 5. Pagamentos e conciliacao

- detalhamento de Pix, debito, credito e dinheiro;
- identificadores de transacao/adquirente quando disponiveis;
- conciliacao por periodo e metodo;
- divergencias e ajustes auditados;
- preparar integracao TEF/adquirentes sem acoplar o dominio a um unico provedor.

## 6. Melhorias operacionais do PDV

- atalhos de teclado;
- busca rapida;
- foco automatico no scanner;
- alteracao de quantidade e remocao eficiente;
- desconto com permissao;
- suspender/retomar venda, se necessario;
- feedback claro de estoque, caixa, offline e erros;
- impressao/recibo nao fiscal enquanto o fiscal real estiver pendente.

## 7. Piloto real

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
