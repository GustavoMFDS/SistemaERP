# PDV — navegação e operação guiadas (2026-10-09)

## Problema identificado

Na interface anterior, a coluna útil do PDV ficava estreita pelo menu lateral ocupar
três colunas de doze; movimentação, fechamento de caixa e seis campos monetários
vinham imediatamente após a abertura do caixa, antes da inclusão de produtos.
O operador precisava percorrer muitos formulários sem distinção clara da tarefa.

## Mudanças implementadas

- Menu lateral mais estreito, com seções **Principal**, **Vendas e clientes**,
  **Produtos e estoque**, **Administração** e destaque da rota atual. No celular,
  os itens seguem roláveis e a navegação explica o gesto de deslizar.
- Área do PDV com largura ampliada, sem a moldura dupla de outras páginas.
- Cabeçalho de orientação e indicadores separados: online/offline, sessão aberta,
  vendas pendentes e itens que exigem atenção.
- Sessão de caixa: um bloco para abrir o turno; quando aberta, exibe identificação
  e suprimento/sangria com explicações breves. O botão de abertura desaparece
  enquanto a sessão já estiver aberta.
- **1. Itens:** leitura de código de barras, busca, seleção e carrinho agrupados.
- **2. Conferir e receber:** coluna lateral de resumo, desconto autorizado,
  pagamento, venda suspensa, finalização e comprovante não fiscal.
- **3. Encerrar turno:** seção própria abaixo da venda, com valores declarados
  organizados em campos de três colunas no desktop e uma coluna no celular.
- Links internos para navegar até produtos, pagamento e fechamento sem caçar
  esses formulários na página.
- Nota explícita de que registrar Pix/cartão **não** significa confirmação real
  do PSP/adquirente. A interface fiscal continua separada.
- Venda offline usa aviso **âmbar** e mensagem de confirmação pendente, não
  uma indicação verde de venda sincronizada. O fluxo de replay segue igual.
- Em telas menores, o fluxo passa para uma coluna e a tabela do carrinho rola
  dentro do próprio componente, sem estourar a largura da página.

Nenhum endpoint ou regra transacional foi alterado nesta entrega. Permanecem
as proteções de idempotência, venda offline em fila, RBAC, descontos e bloqueio
do fechamento com pendências.

## Evidências

- `npm run build` — frontend compilou após reorganização.
- `npm run lint` — sem erros; 10 avisos preexistentes de React Hooks.
- Novo `web/e2e/pdv-layout.spec.ts`: disposição lado a lado no desktop,
  sequência vertical no celular e ausência de rolagem horizontal geral.
- Em banco exclusivo `sistemaemgo_ux`, Playwright Chromium passou em **4/4**
  casos de layout, venda offline/reconexão e operação com carrinho,
  descontos autorizados, atalhos, comprovante e fechamento.
- Mais testes e revisão de acessibilidade/dispositivos reais ainda precisam
  ser executados no HEAD final do PR.

## Próximas melhorias de facilidade

- [ ] Teste presencial com operador de caixa: tempo de venda, confusão entre
      movimentação e fechamento e erro de seleção de meio de pagamento.
- [ ] Identificar e reduzir os 10 avisos de Hooks sem invalidar a proteção
      contra respostas de requisições antigas.
- [ ] Replicar padrões de hierarquia, largura e navegação nas telas de compras,
      inventário, clientes, fiscal e relatórios.
- [ ] Acessibilidade manual com tabulação, leitor de tela, contraste e zoom 200%.
- [ ] Avaliar ajuda inicial em passos, preferências de tamanho de fonte e modo
      de operação em tela cheia para balcões.
- [ ] Isolar os testes Playwright que usam o mesmo caixa, garantindo execução
      integral sem estado compartilhado.

**Não homologado:** esta é uma melhoria de interface e organização, não
integração de pagamento real nem liberação da NFC-e.
