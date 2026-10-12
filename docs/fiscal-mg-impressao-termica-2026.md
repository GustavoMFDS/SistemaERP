# PDV — impressão térmica de DANFE NFC-e (MG, 2026)

## Funcionamento

A impressão do DANFE NFC-e é **opcional**, sempre depois de finalizar a
venda. A obrigação fiscal e a reserva/autorização são independentes da
decisão de imprimir. O botão de impressão do PDV só fica disponível
quando o backend confirmou modelo 65, protocolo, data da autorização,
chave e arquivo do XML processado (`nfeProc`) correspondente.
Não existe impressão fiscal enganosa para notas meramente preparadas,
assinadas ou aguardando resposta da SEF/MG.

A página do PDV verifica automaticamente o resultado fiscal depois da
venda, sem recriar a venda. Quando houver autorização, o funcionário
pode clicar em **Pré-visualizar DANFE**, que abre uma nova aba sem
acionar o diálogo de impressão, ou em **Sim, abrir para imprimir**, que
abre a visualização e solicita ao sistema operacional o diálogo de
impressão. **Não imprimir** apenas fecha a confirmação, sem cancelar
a venda ou qualquer obrigação fiscal.

## Escolha de bobina por caixa

- Com sessão de caixa aberta, escolha **58 mm** ou **80 mm** no campo
  **Bobina deste caixa**, dentro da confirmação da impressão.
- A preferência é salva no **registro do caixa da própria loja** via
  `GET/PUT /api/v1/cash/sessions/{id}/printer`, não no navegador.
  Outro computador da mesma empresa verá a preferência ao utilizar
  esse caixa; outra empresa/CNPJ não pode ler nem alterar a preferência.
- O valor padrão de instalações anteriores é **80 mm**. A migração
  `0048_cash_register_danfe_width` adiciona o CHECK do banco para
  permitir somente 58 ou 80. As modificações possuem auditoria
  transacional `cash.printer.paper_width.updated`.
- Somente usuário autenticado com `cash:open` pode consultar ou
  alterar uma preferência de **sessão aberta** da mesma empresa. A
  alteração usa verificação de origem confiável contra CSRF.
- O HTML é gerado no backend a partir do mesmo registro de caixa da
  **venda original**, incluindo reimpressões pela página Fiscal.
  Não se aceita uma largura arbitrária por parâmetro da URL.
- Configure o driver da impressora com **o mesmo tamanho da bobina**.
  O software fornece HTML/CSS responsivo; não faz transmissão direta
  de comandos ESC/POS, nem seleciona automaticamente impressoras locais.

## Layout e segurança

O template é `internal/modules/fiscal/providers/sefaz/danfe.go`.
O corpo usa no máximo a largura selecionada, margens pequenas e fontes
legíveis em escala térmica. O código preserva cabeçalho, itens, total,
pagamentos, dados fiscais, chave e QR Code v3. O QR Code é rasterizado
em PNG com 256 pixels e ocupa 32 × 32 mm no impresso; cabe nos dois
tamanhos. Regras `break-inside: avoid` tentam evitar separação de
linhas de produto, banners, cabeçalhos e área do QR Code em
quebras de página. O motor do navegador/driver decide as quebras
finais; a qualidade na bobina real deve ser verificada.

A renderização continua recusando nota não autorizada, exceto os
fluxos fiscais separados e explicitamente sinalizados de
cancelamento/contingência já existentes. O PDV nunca apresenta um
documento em contingência pendente como autorizado. O renderer exige
chave e identificadores coerentes com o XML assinado.

**Testes locais:**
- `go test ./...` e `go vet ./...`.
- `TestDANFERendererRendersAuthorizedNFCe` verifica layout 58/80,
  texto escapado corretamente, QR Code PNG, tamanho físico de 32 mm,
  CSS de quebra e rejeição de bobina não suportada.
- PostgreSQL 16 descartável: 48 migrações; teste integrado
  `TestCashRegisterDANFEWidthTenantIsolationAndConstraints` valida
  padrão 80, preferência 58, separação entre empresas e bloqueio de 60 mm.
- Playwright Chromium: `pdv-fiscal-print-choice.spec.ts` exercita
  a seleção da bobina, bloqueio de DANFE não autorizado,
  atualização posterior, pré-visualização sem reemitir venda e
  caracteres UTF-8 na aba HTML.

**Não comprovado:** saída física em modelos específicos de impressora
58/80 mm, sensor de corte, escala efetiva do driver, comportamento
do rolo ao acabar papel, resistência do QR impresso à leitura por
celulares reais ou validade de emissão na SEF/MG. Esses pontos
dependem de um equipamento e de dados de homologação da empresa.
Não ativar produção fiscal apenas porque a impressão HTML passou.


## Endurecimentos adicionais — layout estreito e conferência fiscal

- Em bobinas de **58 mm**, os itens são mostrados em duas linhas: nome
  e descrição ocupam toda a largura; quantidade/preço e total vêm
  abaixo. Em 80 mm, o layout permanece em três colunas proporcionais.
  Isso diminui quebras no meio de nomes extensos e valores monetários.
- A pré-visualização e o botão de impressão ficam temporariamente
  bloqueados enquanto a preferência de bobina está **carregando ou
  salvando**. Se o servidor recusar/falhar ao buscar o tamanho, a tela
  explica o problema e oferece **Tentar carregar bobina novamente**.
  Não imprimir continua disponível em qualquer cenário.
- A rotina compartilhada `FiscalService.RenderNFCeDANFE`, utilizada
  tanto pelo caixa quanto pela página Fiscal, recusa renderizar
  documento marcado `authorized` ou `cancelled` se não existir
  protocolo XML e `nfeProc` não vazios arquivados com a chave de
  acesso da nota. A exceção de contingência offline **pendente**
  permanece distinta e explicitamente sinalizada, sem confundi-la com
  nota autorizada.
- O teste de integração `TestAutoHomologationSaleToSubmittedToAuthorizedWithFakeSEFAZ`
  cria uma situação artificialmente inconsistente (`authorized`
  **sem** `nfeProc`) e comprova que não pode ser impressa. Somente após
  o protocolo fictício ter sido reconciliado/arquivado a renderização é
  permitida.
- Nova regressão visual opcional `web/e2e/danfe-thermal-layout.spec.ts`:
  testa no Chromium **o HTML gerado pelo renderer Go**, com descrições
  compridas, em 58 e 80 mm, mede as larguras reais do corpo, das tabelas
  e do QR (32 mm CSS) e bloqueia layout com rolagem horizontal.
  Para executá-la, defina `DANFE_TEST_BROWSER_DIR` apontando para uma
  pasta descartável, execute o teste Go
  `TestDANFERendererRendersAuthorizedNFCe` para produzir as duas
  amostras fictícias e depois execute o Playwright usando a mesma
  variável de ambiente. Nada é transmitido à SEF/MG.

A visualização na tela é uma verificação **do motor do Chromium**.
Sem uma impressora física e o respectivo driver, não se pode comprovar
o tamanho impresso em papel, guilhotina ou leitura ótica do QR real.
