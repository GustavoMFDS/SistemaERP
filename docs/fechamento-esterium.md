# Fechamento da missão — Esterium ERP

**Atualizado:** 31/10/2026 23:45 BRT (01/11 UTC)
**Estado:** #212 e #213 com CI e laboratório Windows aprovados; #214 CI de integração aprovado; #215 em nova validação após correção do lockfile. Todos os PRs seguem Draft, sem merge ou liberação comercial.

Este relatório registra o estado da cópia em `D:\ERP` e as verificações executadas nesta sessão. Ele não declara o ERP pronto para produção.

## Baseline e limites

- Repositório: `GustavoMFDS/SistemaERP`; baseline consultada: `main` em `69d5e205400d3f4b02c35dca9b164bdeaa9cf49c`.
- A pasta `.git` está somente para leitura nesta sessão. Não contornei o bloqueio nem alterei permissões. Atualizações remotas autorizadas foram feitas pela API do GitHub nas branches das PRs; nenhum commit foi feito localmente.
- Os resultados locais foram organizados fora do repositório. O laboratório usou apenas serviços temporários em loopback, certificado/CA descartáveis e banco de teste nomeado. Os serviços foram encerrados e o banco descartado.
- Não alterei serviços do Windows, arquivo `hosts`, banco real ou instalação do usuário.
- PR #212: head `53f869fd9ea5908ca23a61d9173f2085604a3f7c`, aberta, Draft e sem merge. CI geral passou em 19/19 jobs ([run 41220100576](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41220100576)); a validação integrada Windows passou ([run 41220100486](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41220100486)), somente em runner descartável. Nenhuma instalação foi feita no computador de trabalho; não houve assinatura nem release.

## Trabalho concluído nesta cópia

- Corrigi os PNGs PWA de 192×192 e 512×512 e adicionei `npm run test:assets`, que valida os dois arquivos e confirma a rejeição de uma fixture corrompida. O CI agora executa essa verificação.
- Atualizei a dependência transitiva Zod no lockfile de 4.3.6 para 4.6.7. `npm audit` registrou zero vulnerabilidades; Tailwind permaneceu na linha 3.4.19, sem atualização major.
- Separei as páginas React por rota com `lazy`/`Suspense`. O chunk principal caiu de 545,05 kB (147,81 kB gzip) para 251,11 kB (80,44 kB gzip), redução aproximada de 54% no tamanho minificado.
- Mapeei `web/src` com Graphify: 46 arquivos, 483 nós, 1.119 relações e 13 comunidades. O grafo e as evidências visuais ficaram fora do repositório.

## Verificações locais

- `npm run lint`, `npm run build` e `npm run test:assets`: passaram na repetição final.
- `go vet ./...` passou na auditoria anterior. Nesta continuação, `go test ./...` local foi bloqueado pelo sandbox do Windows ao abrir sockets loopback em cinco testes HTTP; isso não é aprovação nem falha funcional. Os mesmos pacotes são reexecutados no CI hospedado.
- Suíte de integração com PostgreSQL 16.13 e Garnet 2.2.3 descartáveis: 28 testes passaram, nenhum falhou ou foi ignorado; 52 migrações aplicadas e seed de demonstração concluído.
- Playwright em Chromium, Firefox e WebKit: 161 passaram e 6 ficaram condicionais por falta de fixtures/ambiente na execução ampla. Reexecutei os quatro casos de layout DANFE/impressora com fixtures sintéticas e os dois casos prod-like: todos passaram. Cobertura somada: 167 casos, sem falha de produto observada. As execuções focadas ocorreram separadas da suíte ampla.
- O segundo caso prod-like validou dois caixas HTTPS concorrentes: uma venda consumiu a última unidade e a outra recebeu conflito, sem saldo negativo. No Windows, somente as chamadas diretas de `APIRequestContext` usaram o IP loopback, pois esse cliente não herdou a regra DNS do Chromium; a navegação, autenticação, TLS e cookies permaneceram no hostname HTTPS de staging, e as asserções comerciais não mudaram. O teste temporário foi removido.
- A configuração prod-like passou pelo preflight de PostgreSQL com TLS verificado e Garnet com TLS/senha. As credenciais, o certificado e o banco eram descartáveis; isso não equivale a homologação de produção.
- A inspeção visual cobriu as rotas/estados desktop e oito áreas em viewport móvel de 390 px. As oito não apresentaram overflow horizontal e não houve erro fatal do app. A captura de login pegou o fallback de carregamento; ela não é evidência visual válida da tela de login. As imagens e o contato visual foram salvos fora do repositório.
- Bootstrap do primeiro administrador: `go test ./internal/bootstrapadmin ./cmd/esterium-bootstrap-admin` e `go vet` focados passaram; compilação cruzada Windows x64 e parse PowerShell passaram.
- Build de pacote Windows local não terminou: `npm ci` encerrou com erro interno do npm (“Exit handler never called!”). Não foi produzido instalador local; o build do bundle roda também no CI descartável.
- CI geral no SHA `f2aa3542ac4dc42e7182138da196d01066e84eb0` (run [41126937705](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41126937705)): E2E, prod-like, integration, backend, security e a maioria dos jobs Windows passaram; o smoke do PostgreSQL EDB seguia em execução na última consulta.
- CI do instalador Windows no mesmo SHA (run [41126937914](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41126937914)): EXE preview passou; teste integrado ZIP/PostgreSQL ainda estava em execução na última consulta. Não declarar esse job aprovado antes da conclusão.
- CI PR #213 no SHA `39b1d731715a3cf34fa9dabb4178a73ae7283a07`: 19/19 checks passaram (run [41120948782](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41120948782)).
- `git diff --check` local passou antes desta atualização do relatório; será repetido ao finalizar a edição.

## Avanço do instalador Windows

- Commit `f2aa3542` em #212 adiciona seed de RBAC de produção sem usuários/dados de demonstração e bootstrap de uso único para primeiro administrador. O fluxo exige banco local com TLS verificado fora do fixture CI, papel SQL restrito, CNPJ válido, hash bcrypt, senha somente por stdin e transação serializável; valida empresa nova, ausência de dados e recusa replay.
- O teste Windows integrado aplica `seed.prod.sql`, cria administrador sintético, confere perfil/auditoria e login na aplicação, serve React pelo backend em HTTPS sob LocalService, e exercita reinício/recuperação do ERP e Garnet. O resultado depende dos runs acima ainda em execução na última leitura.
- Isso valida componentes e runtime em runner descartável. A versão unsigned de preview ainda bloqueia `-Apply`; o fluxo interativo completo do instalador não foi provado como uma única instalação normal. Reboot integral do Windows, HTTPS confiável em outro equipamento, atualização/reparo/desinstalação assistidos e restore externo continuam pendentes.

## Auditoria das issues abertas

Issue aberta não prova ausência de código. Conferi corpo e código atual; não fechei issues nem publiquei comentários.

- Implementações presentes: #24 variações/SKUs; #35 política de devolução com snapshot na venda; #36 login por usuário/e-mail; #37 lembrete fiscal por perfil/tenant; #38 rota e menu Relatórios; #39 login redireciona a `/home`; #40 convite/ativação sem e-mail obrigatório; #50 recuperação com token de uso único. Permanecem abertas para aceite dos critérios completos, cenários multi-tenant e/ou revisão operacional.
- Parcial: #41 navegação por permissão e áreas secundárias existem, mas falta checklist acessível/responsivo completo; #42 mensagens de venda/fiscal têm salvaguardas em vários fluxos, mas falta auditoria transversal das 19 telas; #26 marca pública Esterium já aparece no frontend, mantendo nomes técnicos e chaves locais antigas para compatibilidade. Aprovação legal de marca continua fora do código.
- #43: Playwright e inspeção cobriram fluxos e oito áreas móveis, mas não há aceite manual registrado de todas as 19 telas, perfis, erros e estados em desktop/celular.
- #44 e #151: componentes reais foram exercitados em CI e o primeiro administrador foi implementado. Ainda falta uma instalação completa pelo fluxo final do pacote, com inicialização automática verificada, reboot integral, HTTPS confiável de segundo terminal, update/reparo, desinstalação conservadora e restore em máquina independente.
- #148 e #194: update de Zod não migrou Tailwind 3 para 4; risco de dependências de build continua pendente de migração visual e audit repetido.
- #29 e #160: integration/E2E passaram uma vez no SHA novo, mas não foi provada repetição da suíte no mesmo banco/ambiente nem reparado eventual vazamento de sessão entre testes.
- #32 e #33: geração/validação XML, filas, reconciliação, idempotência, contingência e montagem de XML processado têm código e testes sintéticos. Não houve transmissão real; protocolo, `nfeProc` e homologação dependem de resposta genuína SEFAZ, certificado e autorização fiscal. Nenhum protocolo foi inventado.
- #28 e #52: são bloqueios históricos de CI/PRs antigas. Os runners atuais executaram jobs reais; isso não reexecuta os HEADs históricos citados nem altera a decisão de não integrar PR antiga isoladamente.
- Critérios externos ainda bloqueiam piloto/produção: Authenticode comercial/proveniência aprovada, certificado e credenciamento SEFAZ, autorização contábil, credenciais Pix/TEF, periféricos, dois computadores físicos, rede e backup externo.

## Critério de conclusão

O fechamento de código e validação automatizada desta rodada foi concluído: CI geral #212 19/19 e fluxo integrado Windows passaram em runner descartável. A execução cobriu instalação ZIP do EXE, TLS/SCRAM, migrações, backup/restauração, recuperação de serviços, telas em Chromium/Firefox, reparo e desinstalação com preservação de segredos e dados. Isso não equivale a homologação de produção. Permanecem necessários: assinatura Authenticode e proveniência do publicador, revisão jurídica das licenças/redistribuição, aceite manual das telas e dos fluxos, validação em dois computadores físicos com reinício/LAN/backup externo, e homologação fiscal com certificado e credenciais reais. Pix/TEF, periféricos e aceite contábil também dependem do ambiente e responsáveis do cliente. Não declarar “ERP pronto para produção”. PRs #212 e #213 continuam separadas, Draft e sem merge; o EXE completo não será distribuído.

## Atualização 30/10/2026 — auditoria de LF da PR #213

- O compare de `main` até o head documental `b9191ad7e2d993f9e1a0f34669635eed40ec9e5d` mostrou apenas oito arquivos esperados, com patches funcionais pequenos; sem regravação integral por fim de linha. Ícones são binários.
- Os quatro arquivos de texto existentes alterados (`.github/workflows/ci.yml`, `web/package.json`, `web/package-lock.json` e `web/src/App.tsx`) têm LF na base e no head, sem CRLF. O validador novo também usa LF. Normalizei para LF o único terminador CRLF no fim deste relatório.
- CI do head anterior `b9191ad7e2d993f9e1a0f34669635eed40ec9e5d`: 19/19 jobs aprovados ([run 41128410265](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41128410265)), incluindo frontend. Esta atualização documental inicia uma nova rodada de CI; aguardando conclusão antes de declarar o head atual verde.
- Nesta cópia local, `npm run test:assets` passou. `npm ci` repetiu o erro interno do npm (“Exit handler never called!”); por isso lint/build locais não puderam iniciar sem `eslint`/`tsc`. A validação correspondente do head remoto anterior passou no CI.


## Atualização 31/10/2026 — CI integrado e reparo seguro aprovados

- A PR #212 recebeu o commit `53f869fd9ea5908ca23a61d9173f2085604a3f7c`. Os scripts de reparo e desinstalação passaram a aceitar a representação canônica do Windows para um único IPv4 `/32` (por exemplo, `127.0.0.1/32` armazenado como `127.0.0.1`). Endereços múltiplos ou outro escopo continuam recusados.
- **CI geral aprovada, 19/19 jobs:** [run 41220100576](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41220100576).
- **Validação do Setup EXE integrada aprovada:** [run 41220100486](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41220100486). O teste usou PostgreSQL ZIP extraído do EXE em runner efêmero; validou TLS verify-full/SCRAM, migração até a versão 52, restauração de backup em cluster independente e conta ERP sem privilégios administrativos.
- O primeiro administrador sintético foi criado sem conta ou catálogo de demonstração. O serviço ERP LocalService serviu a aplicação React por HTTPS; login sintético e perfil passaram. As 17 telas abriram em Chromium e Firefox; a matriz móvel não teve overflow horizontal.
- Passaram parada/início do ERP, recuperação de falha forçada, reinício de Garnet, reparo com bytes de segredo/TLS preservados e desinstalação que removeu apenas aplicativo/serviço/firewall/atalho, arquivando recibos e preservando serviços de dependência, segredos e dados SQL.
- A suíte de Garnet validou TLS, ACL, Lua atômico, AOF após falha e NSSM. A transmissão SEFAZ permaneceu desligada; foram usados ambiente e dados sintéticos.
- Na execução Windows, os jobs manuais independentes foram ignorados por serem disparados só manualmente; o job de integração e o job da prévia foram aprovados. Não se fez instalação em máquina de cliente. Authenticode, revisão jurídica, aceite manual, rede física/backup externo e homologação fiscal continuam pendentes.


## Atualização 31/10/2026 — revalidação técnica dos PRs

### PR #212 — head `971f0a7be40cb609bfa6dae69db1e58bb631b812`

- CI geral aprovou **19/19 jobs** ([run 41225927554](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41225927554)); `go test ./...`, `go vet ./...`, build, lint, integração e audit passaram.
- Playwright passou **162/162 duas vezes no mesmo SHA**; a segunda execução do job E2E foi concluída com sucesso no run acima. A fixture exige `E2E_DISPOSABLE_DATABASE=1`, restringe a API a loopback, consulta `expected_cash` sob lock e fecha no valor reconciliado. O teste sintético confirmou abertura de R$ 25 + venda em dinheiro de R$ 10 = saldo esperado R$ 35, com diferença zero no teardown.
- Windows integrado aprovou no runner descartável ([run 41225927628](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41225927628)): PostgreSQL ZIP do próprio EXE, initdb não administrativo, TLS verify-full/SCRAM, schema v52, restore em cluster independente, primeiro admin sintético, ERP LocalService/HTTPS, Garnet TLS/ACL/AOF/NSSM, recuperação, reparo e desinstalação.
- Telas 18/19 passaram em Chromium, Firefox e Chromium mobile; as 17 telas abriram em Chromium/Firefox, sem overflow horizontal. Capturas estão no artefato `esterium-windows-lab-screens` do run Windows, válido até 07/11/2026.
- A issue #160 segue aberta: isolamento seguro e duas execuções ficaram cobertos; o primeiro teste que deixa sessão aberta ainda não foi identificado e o aceite da issue não foi registrado.

### PR #213 — código no head `ac112b36bdc205da024cfa611b0ee5776e498463`

- CI aprovou **19/19 jobs** no [run 41223453520](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41223453520); E2E: 161/161. Esta referência valida o código antes desta atualização documental.
- `npm audit --audit-level=moderate` reportou 0 vulnerabilidades. `govulncheck` reportou 0 vulnerabilidades alcançáveis; manteve 7 achados em dependências e 1 módulo não chamado. O lock fixa `minimatch 11.0.1` (corrige [GHSA-g3hv-75wg-m249](https://github.com/advisories/GHSA-g3hv-75wg-m249)); Tailwind permanece 3.x.

### Issues e limites

As issues #24, #28, #29, #32, #33, #35, #36, #43, #52, #160 e #194 seguem abertas; nenhuma foi fechada ou comentada. Testes sintéticos não substituem aceite manual de todas as telas, homologação SEFAZ, Pix/TEF, periféricos ou contabilidade.

Ambos os PRs permanecem Draft e separados. Não houve merge, release, instalação em máquina de cliente ou reinício completo do Windows como sistema operacional em máquina real. Permanecem revisão jurídica de redistribuição, assinatura/proveniência Authenticode, validação física em dois PCs com LAN/backup/reboot e credenciais/homologação fiscal reais. A prévia não está pronta para produção.

## Revalidação 31/10/2026 — sequência completa #212

- PR #213 head `921afc18e1e981a0cc211615c2a3f621ea8e0a49`: CI **19/19** aprovado ([run 41232516290](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41232516290)); E2E: 161 passaram, 6 ignorados pelas condições da suite; smoke do PostgreSQL EDB 16.15 também passou. O teste de legenda agora espera a resposta PATCH antes de verificar estado persistido.
- PR #212 head `fca67763998dbe2ff917269e6ba0d8047a5c19aa`: CI **19/19** aprovado ([run 41234145814](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41234145814)). A segunda execução E2E no mesmo SHA foi solicitada e ainda estava ativa na última consulta; não contar como concluída antes do resultado.
- O job do instalador integrado segue em andamento no runner Windows descartável ([run 41234145812](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41234145812)). Diagnóstico anterior isolou falha no bootstrap do primeiro administrador: o coordenador deixava PostgreSQL parado/desabilitado após migrations, mas tentava abrir conexão para criar o admin. A correção inicia PostgreSQL apenas durante o bootstrap, depois para/desabilita e verifica o estado antes do probe. O job atual ainda precisa aprovar para confirmar a correção.
- Nenhum serviço ou dado do Windows local foi alterado. Não houve instalação em cliente, merge, release, venda ou ativação fiscal.

### Bloqueios externos mantidos

Reboot real em Windows limpo, validação física em dois PCs/LAN, restore externo independente, assinatura comercial Authenticode e proveniência do editor, revisão jurídica dos fornecedores, transmissão SEFAZ real, Pix/TEF, periféricos, aceite contábil e aceite do responsável pela issue #160 seguem pendentes. A issue #160 permanece aberta: não foi identificada a origem histórica da primeira sessão vazada.

### Atualização de CI posterior — head atual #212 `307134c2ebcfd1b11b27f1d6cc33e0fa501a2e77`

- PR #212 CI geral: **19/19 jobs aprovados**, incluindo EDB 16.15 ([run 41239727842](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41239727842)).
- E2E no head atual passou: 162 testes e 6 ignorados; a segunda execução no mesmo SHA está em andamento.
- A bancada integrada ainda está em execução ([run 41239727820](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41239727820)). A execução do head anterior falhou com diagnóstico `ancestor` e HTTP 429 no login final; o head atual normaliza o nome seguro e aguarda a janela Redis antes da verificação final.
- Parser local dos três scripts alterados passou. Não declarar a instalação integrada aprovada até terminar instalação, navegação HTTPS, recuperação, reparo e desinstalação.
- PR #213 head documental atual `1289e03c891fa192d1656ee40773f562d84a8b49`: CI em execução ([run 41239760021](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41239760021)).
- Sem alteração no Windows local, instalação em cliente, merge, release ou ativação fiscal.

### Bloqueios externos mantidos

Reboot real em Windows limpo, validação física em dois PCs/LAN, restore externo independente, assinatura comercial Authenticode e proveniência do editor, revisão jurídica dos fornecedores, transmissão SEFAZ real, Pix/TEF, periféricos, aceite contábil e aceite do responsável pela issue #160 seguem pendentes. A issue #160 permanece aberta: a origem histórica da primeira sessão vazada não foi identificada.


## Atualização 31/10/2026 — correção da lista PostgreSQL na PR #212

- Na instalação completa do head `307134c2ebcfd1b11b27f1d6cc33e0fa501a2e77`, o probe recebeu um único caminho combinado, `OriginalPostgresql-EsteriumPostgreSQL16.conf_OriginalPgHba-EsteriumPostgreSQL16.conf`, devido à expressão PowerShell que montava os dois nomes. A causa foi reproduzida.
- O head `61d26f0b6b76d58f1ff0d853bde5bc90ea781b39` separa as duas entradas e testa que existam exatamente duas; o teste estático também rejeita a expressão anterior. Os dois scripts passaram pelo parser PowerShell local.
- No head anterior, CI geral passou 19/19 e E2E passou duas vezes (162 passaram, 6 ignorados por execução). No run Windows `41239727820`, a prévia ZIP passou instalação, TLS/SCRAM, migrations, recuperação, navegador, cooldown, reparo e desinstalação conservadora; a instalação completa falhou somente ao encontrar o caminho combinado.
- Para o novo head, a prévia unsigned já passou no run Windows `41241089612`. Os jobs de instalação completa e ZIP ainda estão em execução; o CI geral `41241089597` também permanece em execução na última consulta. A correção aguarda validação integrada.
- PR #213 antes desta atualização documental estava no head `853788f08a79e39f4d4a213f2096b11bc70cd556`; seu CI `41240481987` ainda aguardava o smoke do instalador PostgreSQL EDB 16.15. Esta alteração inicia nova validação documental.
- PRs #212 e #213 seguem abertas e Draft. Sem merge, release, instalação em cliente ou ativação fiscal/comercial.
## Atualização 31/10/2026 — bancada Windows #212 validada

- PR #212 head `b1c68c9caa7e1f0d15d1ec27d89de2ab8295c69b`: CI [run 41254136873](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41254136873) concluiu **19/19 jobs sem falhas**.
- E2E do mesmo head passou duas vezes, cada uma com **162 aprovados e 6 ignorados**.
- Bancada [Windows Setup EXE run 41254136879](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41254136879): `unsigned-preview`, `unsigned-integrated-zip-preview`, `complete-first-install-lab` e `oss-component-gate` passaram.
- Instalação completa em runner descartável validou o recibo de prontidão, 17 telas em Chromium e Firefox com TLS confiável, rotas móveis sem overflow, operações sintéticas, reinício do ERP e dependências, recuperação de falha do serviço, reparo assistido e desinstalação conservadora.
- Reparo restaurou política SCM, regra HTTPS e atalho sem alterar arquivos da aplicação, banco, segredos, TLS ou backup. Desinstalação removeu app/serviço/regra/atalho e arquivou recibos, mantendo PostgreSQL, Garnet, segredos e dados SQL.
- ZIP integrado validou fontes MIT extraídas do Setup EXE, PostgreSQL 16 TLS verify-full/SCRAM, migrations até schema 52, restauração de backup, recuperação pós-falha e operação ERP local. XML foi gerado apenas no modo local sintético; SEFAZ permaneceu desativada.
- PostgreSQL EDB 16.15 teve hash e Authenticode de fornecedor confirmados, instalação unattended, serviço/conta dedicada e parada segura aprovados.
- A assinatura da instalação completa veio de certificado efêmero de CI para exercício técnico. Isso não comprova Authenticode comercial, proveniência comercial, reinicialização física em cliente, LAN real, restore externo, SEFAZ, Pix/TEF, periféricos nem aceite contábil.
- PR #212 segue aberta e Draft, sem merge/release/instalação em cliente. A issue #160 permanece aberta para acompanhar estabilidade e possíveis teardowns adicionais; a corrida original foi identificada e corrigida pela #161 (`cash-movement-reload.spec.ts`, commit `759992f4`).
- O CI do head documental `0c66f6e7600e98d2edcbd65c5cfddf597a06fd96` concluiu 19/19 jobs ([run 41258963854](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41258963854)); E2E 162 passaram e 6 foram ignorados; EDB 16.15 passou.
- Esta correção do relatório registra os resultados acima e inicia CI próprio para a nova revisão documental. A missão de liberação continua condicionada aos bloqueios externos listados acima.

## Atualização 01/11/2026 — fila offline e revalidação

- Issue #160: a corrida histórica foi localizada em `cash-movement-reload.spec.ts` e corrigida pela PR #161, merge `759992f4`. A issue segue aberta para observação de estabilidade e outros gaps de teardown, conforme comentários do responsável.
- PR #212 head `5e061510d4e7babbe6f25807ec803f26787ce277`: CI [run 41260683054](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41260683054) aprovou **19/19 jobs**, incluindo o smoke do PostgreSQL EDB 16.15. Playwright passou com **164 aprovados e 6 ignorados**.
- A fixture E2E agora exige ausência de sessão anterior no começo do teste. No teardown, reconecta o browser, tenta reconciliar fila offline/legada e só fecha caixa quando ambas estão vazias; falha de reconciliação deixa a sessão aberta. A regressão sintética confirmou que a venda offline chegou ao servidor antes do fechamento.
- A bancada Windows do mesmo head [run 41260683199](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41260683199) ainda estava validando instalação completa e ZIP integrado no momento desta atualização. A última bancada completa aprovada continua sendo run 41254136879, no head anterior `b1c68c9`.
- PR #213 head documental `7d19fc7d25d7eeaf8bbfef87c203c7a163e44103` concluiu CI **19/19** ([run 41259691614](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41259691614)); este novo registro inicia validação documental própria.
- PRs #212 e #213 seguem Draft. Sem merge, release, instalação em cliente ou ativação fiscal/comercial. Reboot real, rede física, restore externo, assinatura comercial/proveniência, revisão jurídica, SEFAZ, Pix/TEF, periféricos e aceite contábil seguem pendentes.

## Fechamento técnico — 01/11/2026

- PR #212 head `5e061510d4e7babbe6f25807ec803f26787ce277`: CI [run 41260683054](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41260683054) aprovou **19/19 jobs**, incluindo PostgreSQL EDB 16.15; E2E passou com **164 testes e 6 ignorados**.
- Bancada Windows [run 41260683199](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41260683199) concluiu com sucesso. `unsigned-preview`, `unsigned-integrated-zip-preview`, `complete-first-install-lab` e `oss-component-gate` passaram; jobs opcionais independentes foram ignorados conforme configuração.
- A instalação integrada criou primeiro administrador sintético, serviu HTTPS com confiança do sistema e abriu as 17 telas em Chromium/Firefox. A matriz móvel não teve overflow; operações sintéticas de catálogo, compras, caixa, venda, devolução, financeiro, XML local e concorrência em dois terminais passaram. Parada/início, recuperação de falha do ERP e reinício de Garnet também passaram.
- A reparação recuperou apenas política SCM, firewall HTTPS, atalho e saúde; arquivos de aplicação, SQL, segredos e TLS ficaram preservados. A desinstalação arquivou recibos e removeu somente ERP/serviço/regra/atalho, preservando PostgreSQL, Garnet, segredos e dados.
- ZIP integrado usou componentes verificados extraídos do EXE; PostgreSQL 16.15 passou TLS verify-full/SCRAM, migrations até schema 52, restore em cluster independente e recuperação. Garnet passou TLS/ACL/Lua/AOF; o executável ERP real foi registrado como serviço CI parado/manual e removido ao final.
- Evidências de navegador: [instalação completa](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41260683199/artifacts/13065063494) e [bancada ZIP](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41260683199/artifacts/13065343117). A assinatura da instalação completa foi efêmera de CI; não é assinatura comercial.
- A fixture E2E exige que não haja sessão de teste anterior aberta. No teardown, reconecta browser, reconcilia filas pendentes/legadas e só fecha caixa com filas vazias; falha em reconciliação deixa caixa aberto. A regressão provou que venda offline chega ao servidor antes do fechamento.
- Issue #160 continua aberta para observar estabilidade e possíveis gaps restantes. A causa histórica foi identificada em `cash-movement-reload.spec.ts` e corrigida pela PR #161 (merge `759992f4`); não é mais correto descrevê-la como desconhecida.
- Limites: runner não fez reboot do Windows como sistema operacional, piloto físico em dois PCs/LAN, restore externo, aceite de fornecedores/periféricos, transmissão SEFAZ real, Pix/TEF, Authenticode comercial/proveniência ou aceite contábil.
- PRs #212 e #213 seguem abertas e Draft. Sem merge, release, instalação em cliente ou ativação fiscal/comercial. Head documental anterior `7d19fc7d25d7eeaf8bbfef87c203c7a163e44103` passou CI 19/19 em [run 41259691614](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41259691614); esta revisão final inicia CI próprio.## Atualização 31/10/2026 — revalidação e auditoria final das issues

### Estado dos PRs e verificações

- **PR #212 — Windows local:** head cbbd72eb62ffcc1f32d629f1dcbb7b7e125030fe. CI geral [run 41263732350](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41263732350) e bancada Windows [run 41263732319](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41263732319) concluíram com sucesso. O laboratório cobriu instalação, reparo, desinstalação e exercício de atualização somente do aplicativo, rollback e recusa de migração insegura em runner descartável.
- **PR #213 — fechamento/PWA:** head 237887548d835fc153cc35b75b9972b00d59df. A segunda tentativa do CI [run 41264814572](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41264814572) concluiu **19/19 jobs**; E2E passou com **161 testes e 6 condicionais**. Frontend, backend, integração, segurança e bancada Windows passaram.
- **PR #214 — repetição de integração:** head 43de231888979917e52823932d2ec7429fac4c8a. CI [run 41264794584](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41264794584) concluiu com sucesso em todos os jobs, incluindo repetição de integração no mesmo banco sintético após isolamento de tenant e preservação da associação de usuário exigida pelo histórico fiscal imutável.
- **PR #215 — Tailwind 4:** head 785122a52a8362f470091ddf35cf202f91fafe09. O primeiro CI [run 41265424626](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41265424626) apontou lockfile fora de sincronia no Linux, pedindo duas dependências transitivas. A correção foi commitada no ramo e o CI novo [run 41265992945](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41265992945) está em execução nesta leitura. Ainda não declarar a migração aprovada até esse run terminar. A alteração mantém o suporte mínimo documentado pelo Tailwind 4 (Safari 16.4, Chrome 111 e Firefox 128); a inspeção visual final também depende da nova execução.
- Os quatro PRs continuam abertos em Draft. Nenhuma issue foi fechada automaticamente.

### Mapa atualizado das 22 issues abertas

Verificação feita ao vivo no GitHub em 31/10/2026: as 22 issues abaixo continuam abertas. “Implementado” indica evidência em código/teste; não equivale a aceite do responsável nem liberação de produção.

| Issue | Situação verificada | Pendência para fechar |
|---|---|---|
| #194 — alertas de build/Tailwind | Migração para Tailwind 4 preparada no #215; CI novo ainda em execução. | CI verde, revisão visual e confirmação da política de suporte a navegadores. |
| #160 — isolamento de caixa E2E | Fixture exige sessão limpa, reconcilia fila antes de fechar e falha com venda pendente; regressão passou. | Manter aberta para observar estabilidade e outros casos de teardown. |
| #151 — checklist de release não fiscal | Instalação e recuperação exercitadas em runner descartável no #212. | Authenticode comercial, reboot real, dois PCs/LAN, restore externo e aceite operacional. |
| #148 — segurança do frontend/Tailwind | Mesma migração do #194, em validação no #215. | Aprovação final de CI, dependências, navegadores e visuais. |
| #52 — CI histórico da PR #25 | Os runners atuais funcionam; a cadeia histórica permanece documentada. | Revisão isolada dos HEADs históricos; não integrar PR antiga sem nova decisão. |
| #50 — recuperação sem e-mail | Token de uso único e fluxo sintético têm implementação e cobertura. | Aceite do processo presencial e validação operacional de recuperação. |
| #44 — instalação local Windows | Pacote local e laboratório integrado avançaram e passaram no runner. | Validação física, reboot do Windows, rede real, assinatura comercial e restore externo. |
| #43 — UX das 19 telas | Requisitos mapeados em 17 rotas e 2 fluxos transversais; Playwright e laboratório cobrem rotas/estados principais. | Aceite manual completo por perfil, erros e tamanhos de tela. |
| #42 — mensagens e confirmações | Confirmação da venda e estados fiscais foram exercitados; há salvaguardas em fluxos sensíveis. | Auditoria transversal de mensagens, erros, offline e confirmação nas telas. |
| #41 — navegação e atenção | Navegação principal/secundária, central de atenção e lembrete fiscal por perfil foram exercitados. | Aceite de acessibilidade, restrições de caixa e estados de atenção restantes. |
| #40 — ativação de acesso | Ativação por código sem e-mail obrigatório está implementada e coberta. | Aceite operacional dos convites e recuperação de acesso. |
| #39 — login | Login por usuário e redirecionamento para /home estão implementados e cobertos. | Aceite visual e operacional final. |
| #38 — relatórios | Rota e item Relatórios separados do Financeiro existem. | Conferir critérios e indicadores com o responsável do negócio. |
| #37 — lembrete fiscal | Lembrete por perfil/tenant e sua dispensa foram cobertos no fluxo de início. | Confirmar comportamento final com o responsável fiscal. |
| #36 — funcionário sem e-mail | Cadastro/login com identificador de usuário sem e-mail está implementado e coberto. | Aceite do fluxo administrativo e de suporte. |
| #35 — política de devolução | Política por produto e snapshot na venda/devolução estão cobertos por testes. | Aceite operacional das regras e exceções. |
| #33 — protocolo SEFAZ/nfeProc | Guards, arquivo e builders locais usam apenas dados sintéticos. | Resposta genuína SEFAZ, certificado, autorização e validação contábil. |
| #32 — emissão automática após venda | Fila, idempotência e reconciliação local foram exercitadas; emissão real não ocorreu. | Homologação e credenciamento SEFAZ; não habilitar transmissão sem autorização. |
| #29 — rerun de integração | #214 passou na repetição do mesmo banco descartável com isolamento de tenant. | Manter a execução repetível como gate e ampliar observação de estado residual. |
| #28 — runners de Actions | CI recente executou jobs Linux/Windows reais e passou nos PRs indicados. | Revalidar os HEADs antigos citados na issue se ainda forem necessários. |
| #26 — rebranding | Marca Esterium aparece nas superfícies públicas; nomes internos/chaves antigas foram preservados por compatibilidade. | Revisão jurídica de marca, domínio, embalagem e materiais públicos. |
| #24 — variações/estoque | Variações/SKUs e estoque por opção têm migração, integração e cobertura E2E. | Aceite de importação, sincronização offline e limpeza de variação. |

### Graphify — escopo web/src

- Grafo gerado com 46 arquivos, 483 nós, 1.119 relações e 13 comunidades. Relatório e artefatos locais: D:\ERP\web\src\graphify-out\GRAPH_REPORT.md, graph.json e graph.html.
- Nós centrais: errorMessage() (75 ligações), apiJson() (63), getSessionScope() (45), PDVPage() (43) e scopedStorageKey() (24).
- Conexão que merece revisão: apiJson() liga várias comunidades, com intermediação 0,279. Pergunta sugerida: tratamento e normalização de respostas da API devem formar uma fronteira compartilhada explícita?

### Critério de conclusão

O fechamento técnico em CI avançou e a cópia local contém relatório e grafo reproduzível. A migração Tailwind #215 ainda aguarda o CI da correção do lockfile. A missão de produção continua incompleta: não houve instalação em cliente, Authenticode comercial, reboot físico, aceite de dois PCs/LAN e restore externo, homologação SEFAZ real, ativação Pix/TEF, validação de periféricos, revisão jurídica nem aceite contábil. Manter os PRs em Draft e não declarar o ERP pronto para produção até resolver os gates técnicos e externos aplicáveis.


## Atualização 01/11/2026 00:06 BRT — validação final desta rodada

Esta seção substitui os estados pendentes das atualizações anteriores. CI final e artefatos foram consultados no GitHub em 01/11/2026.

### Estado dos quatro PRs

- **#212 — instalador Windows:** head `cbbd72eb62ffcc1f32d629f1dcbb7b7e125030fe`. CI [41263732350](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41263732350) passou em **19/19 jobs**. Bancada Windows [41263732319](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41263732319): instalação completa, ZIP integrado, preview unsigned e gate OSS passaram; quatro verificações opcionais independentes foram ignoradas pela configuração. Playwright: **164 passaram, 6 ignorados**.
- **#213 — fechamento e PWA:** head de código `fa93048dd1774425693d06538a68aef70bb77364`. CI [41266126981](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41266126981) passou em **19/19 jobs**, incluindo EDB PostgreSQL; Playwright: **161 passaram, 6 ignorados**. Esta atualização documental aciona nova rodada de CI para o head que contém este relatório.
- **#214 — repetição segura da integração:** head `43de231888979917e52823932d2ec7429fac4c8a`. CI [41264794584](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41264794584) passou em **19/19 jobs**; Playwright: **161 passaram, 6 ignorados**. As suítes repetiram no mesmo banco fiscal descartável, preservando o histórico imutável.
- **#215 — migração Tailwind 4:** head `73fbaef2df39cda8c7ffc64e2b29dbfbd17c2fad`. CI [41267040692](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41267040692) passou em **19/19 jobs**; Playwright: **162 passaram, 6 ignorados**. `npm audit --audit-level=moderate` encontrou zero vulnerabilidades; o scan do código não identificou vulnerabilidade alcançável (sete achados de dependências e um módulo não chamado permanecem no relatório de segurança).
- Os quatro PRs estão abertos e em Draft. Nenhuma issue foi fechada nesta atualização.

### Evidência visual e desempenho

- Artefato visual do CI: [cinco capturas Tailwind 4](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41267040692/artifacts/13067646590). Login, estoque, catálogo, PDV desktop e PDV móvel foram revisados. A captura móvel do PDV não mostra rolagem horizontal geral; a tabela do catálogo mantém rolagem horizontal própria para colunas largas.
- Artefato de auditoria de dependências: [frontend-audit](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41267040692/artifacts/13067946540).
- O build do #215 ainda emite aviso de chunk principal de aproximadamente 545 kB nessa branch baseada em `main`. A divisão de rotas da #213 reduz o chunk principal de aproximadamente 545 kB para 251 kB (gzip: 147,81 kB para 80,44 kB); a otimização está em PR separada.
- Tailwind 4 requer Safari 16.4+, Chrome 111+ e Firefox 128+ segundo o [guia oficial de atualização](https://tailwindcss.com/docs/upgrade-guide). Confirmação da política mínima de navegadores pelos responsáveis do produto segue necessária.

### Pendências que impedem declarar produção

As 22 issues da auditoria permanecem abertas; em particular, #148 e #194 aguardam aceite de navegador/visual e responsável, e #160 permanece aberta para observação de estabilidade. Ainda faltam assinatura comercial Authenticode e proveniência, reboot em Windows limpo, validação física em dois PCs/LAN, restore externo independente, revisão jurídica de marca/licenças, credenciais e homologação SEFAZ genuínas, aceite contábil, Pix/TEF, periféricos e aceite operacional das telas/fluxos. Não houve instalação em cliente, transmissão fiscal real, merge ou release. Manter o ERP classificado como não homologado para produção.


## Atualização 01/11/2026 — auditoria da instalação integrada e validação restante

### Evidências executadas em runner Windows descartável

- PR #212, head `cbbd72eb62ffcc1f32d629f1dcbb7b7e125030fe`: CI [41263732350](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41263732350) passou em 19/19 jobs; bancada [41263732319](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41263732319) aprovou instalação completa, ZIP integrado, prévia e gate OSS. Quatro jobs opcionais foram ignorados conforme configuração.
- A bancada compilou o bundle Windows, aplicou assinaturas de teste temporárias aos arquivos Esterium, baixou EDB PostgreSQL 16.15, Garnet 2.2.3 e NSSM com hashes fixados, verificou o publicador Authenticode do EDB e executou o coordenador `install-esterium-local-complete.ps1 -Apply` num runner limpo.
- O ERP foi iniciado como serviço LocalService; PostgreSQL usou conta restrita; Garnet usou NetworkService. A bancada confirmou migrations, conta admin sintética sem replay, credenciais protegidas, Garnet TLS/ACL/AOF, HTTPS confiável pelo sistema, React servido pelo backend, health, regra de firewall limitada a um endereço privado e atalho.
- Playwright contra os serviços instalados executou 10 casos no Windows: 6 passaram e 4 foram ignorados por escopo de projeto. Cobriu as 17 rotas reais, os estados aprovados de tela 18 (lembrete fiscal/navegação/alertas) e tela 19 (confirmação da venda), Chromium, Firefox e viewport móvel. Operações sintéticas cobriram equipe/permissões, produto/foto/variante/estoque, fornecedor/compra/recebimento, cliente, venda e idempotência, troco, caixa, devolução/reembolso, contas/relatórios, XML fiscal local e disputa de estoque entre dois terminais. Não houve transmissão SEFAZ. Evidência: [capturas da instalação integrada](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41263732319/artifacts/13067171264).
- Restart de PostgreSQL/Garnet/ERP, recuperação após falha do processo ERP, reparo assistido e desinstalação conservadora passaram; segredos, TLS, PostgreSQL/Garnet e dados SQL permaneceram preservados. Atualização e rollback do aplicativo passaram em fixture sintética sem alterar banco/SQL.
- O job ZIP integrado restaurou um arquivo PostgreSQL 16 em cluster Windows independente e comparou migrations, conteúdo e tabelas críticas. Os jobs de backup exercitaram integridade/configuração e cópia externa sintética; nenhum share SMB externo foi disponibilizado para restaurar um backup de loja.
- A instalação de serviço foi exercitada pelo coordenador real extraído do bundle. O fluxo interativo do EXE comercial não foi usado para executar Apply; as verificações de prévia/EXE e o caminho do coordenador foram validados separadamente. A assinatura temporária de CI não é Authenticode comercial.

### Integridade do diff da PR #213

- Os arquivos de texto existentes que mudaram na branch mantêm LF como na base; nenhum deles ganhou CRLF. Arquivos novos do relatório e do validador de ícones também usam LF. O patch contém mudanças funcionais/localizadas em CI, PWA, lockfile e carregamento por rota, sem regravação de arquivos.
- A auditoria do patch encontrou um espaço final na linha de data deste relatório; ele foi removido nesta atualização. A CI do novo head documental será registrada na descrição da PR após concluir.

### Validação que depende de VM/laboratório externo

- Não consegui acessar uma VM Windows nesta sessão: `Get-VM` falhou por indisponibilidade do recurso CIM e o Docker Desktop não tinha daemon acessível. O runner hospedado executou o laboratório descartável, mas foi encerrado sem reinicializar o sistema operacional.
- Continua pendente o ciclo de reboot integral: instalar, reiniciar o Windows e confirmar inicialização automática PostgreSQL/Garnet/ERP, readiness HTTPS, login e persistência dos dados sintéticos após boot. Também seguem pendentes segundo dispositivo físico/LAN e restauração a partir de share externo independente.
- Piloto/produção continuam bloqueados por Authenticode/proveniência comercial, revisão jurídica, aceite de navegador/telas/contabilidade, credenciamento/certificado SEFAZ genuínos, Pix/TEF e periféricos. Não instalar em cliente, transmitir documento real, mesclar PR ou liberar vendas.

## Atualização 01/11/2026 00:45 BRT — fechamento das rodadas atuais

### PR #212 — instalador Windows integrado

- Head `5813bf7cab5ff75c2c5d36b50e7c5f1a87d30eb2`, aberta e Draft. CI geral [run 41269111509](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41269111509) concluiu **19/19 jobs**.
- O E2E executou 170 casos. Na primeira tentativa, 163 passaram, 1 falhou e 6 foram ignorados: o caso de duplo clique não encontrou a confirmação de venda em cinco segundos. Reexecutei somente o job no mesmo SHA, sem mudanças no código; o rerun passou com **164 testes e 6 ignorados**. A falha não se repetiu; manter #160 aberta para acompanhar estabilidade.
- Bancada Windows [run 41269111620](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41269111620): os 4 jobs configurados passaram (prévia, instalação completa, ZIP/restauração e gate de componentes); 4 verificações opcionais ficaram ignoradas pela configuração.
- A bancada construiu um `EsteriumERP-Setup.exe` com o bundle real do ERP e EDB PostgreSQL, Garnet e NSSM aprovados por hash, usando identidade temporária de assinatura do CI. Authenticode do teste e probes de payload/modo retail passaram. Isso não é Authenticode comercial.
- A instalação completa executou o coordenador real com `-Apply` em runner limpo; criou os serviços e administrador sintético, aplicou migrations, TLS/SCRAM/ACL/AOF, HTTPS confiável, React servido pelo backend, health, firewall restrito e atalho. Playwright nos serviços instalados: 6 passaram e 4 foram ignorados conforme os projetos; 17 rotas abriram em Chromium e Firefox, sem overflow horizontal na matriz móvel; operações sintéticas de equipe, catálogo, compras, caixa, venda, retorno, financeiro, XML local e disputa de estoque passaram.
- Reinício/recuperação de ERP, PostgreSQL e Garnet, reparo assistido, desinstalação conservadora e fixtures de update/rollback do aplicativo passaram. A restauração do arquivo PostgreSQL ocorreu em segundo cluster Windows independente e conferiu migração, conteúdo e tabelas críticas.
- Evidências: [navegadores da instalação integrada](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41269111620/artifacts/13067909951), [capturas Windows](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41269111620/artifacts/13068718231), [prévia técnica unsigned](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41269111620/artifacts/13068367654).
- A instalação foi aplicada pelo coordenador extraído do pacote; o clique interativo do botão Apply do Setup não foi automatizado. O runner não reiniciou o Windows como sistema operacional. Continuam pendentes VM acessível, teste de reboot integral, segundo dispositivo/LAN e restore de share externo, além dos gates comerciais/fiscais já listados.

### PR #213 e estado geral

- Head #213 `070e14aaa50cf2aaae727f56cc16a1766ddcf520`: CI [run 41269053863](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41269053863) passou em **19/19 jobs**; E2E: **161 passaram e 6 foram ignorados**. A auditoria de LF segue sem conversão CRLF ou whitespace excedente.
- Este registro documental aciona uma nova CI para o head atualizado da #213; a descrição da PR será alinhada ao resultado dessa rodada após a conclusão.
- PRs #212, #213, #214 (`43de231888979917e52823932d2ec7429fac4c8a`) e #215 (`73fbaef2df39cda8c7ffc64e2b29dbfbd17c2fad`) permanecem abertas e Draft. A auditoria ao vivo encontrou **22 issues abertas**; nenhuma foi fechada.
- `git diff --check` local não encontrou erro de whitespace; houve apenas avisos de autocrlf em três arquivos locais. O relatório local não contém espaços finais. Nenhum merge, release, instalação em cliente ou transmissão fiscal foi feito.

## Adendo 01/11/2026 — integração #216 validada

**Código validado:** `8fffcee1bff0728463c757aa8aafc90c1f4a2348`, baseado em `main` `69d5e205400d3f4b02c35dca9b164bdeaa9cf49c`. [PR #216](https://github.com/GustavoMFDS/SistemaERP/pull/216) segue aberta e Draft. As PRs #212–#215 também seguem abertas e Draft; nenhuma foi mesclada.

### CI

- [CI completo](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41288736318): **20/20 jobs aprovados** no SHA acima.
- Duas suítes E2E independentes, cada uma em banco descartável: **167 passaram, 6 ignorados** de 173 testes. [Evidência run 1](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41288736318/artifacts/13076248239) · [evidência run 2](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41288736318/artifacts/13076820367).
- Integração PostgreSQL/fiscal/fornecedores repetida na mesma base descartável passou; restauração também foi verificada em base independente. O smoke Windows instalou o PostgreSQL 16.15 EDB oficial, conferiu SHA-256 e assinatura Authenticode do fornecedor e encerrou serviço/porta com segurança.
- Build, lint, assets, marca, Go tests/vet e SPA same-origin passaram. `npm audit` encontrou zero vulnerabilidades; `govulncheck` encontrou zero vulnerabilidades alcançadas no código. Registrou 7 alertas em pacotes importados e 1 em módulo requerido que não são chamados pelo código analisado; manter issue #194 para revisão da política. [Relatório de auditoria](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41288736318/artifacts/13076735492).

### Instalador e primeira instalação Windows

- [Workflow Windows Setup EXE](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41288736458): os quatro gates aplicáveis à PR — prévia, pacote integrado, primeira instalação e gate agregado — passaram. Quatro jobs reservados a `workflow_dispatch` foram ignorados por não serem executados em PR.
- O pacote integrado verificou 379 arquivos e hashes, PostgreSQL 16.15 real, TLS verify-full/SCRAM, serviço Windows não administrador, migrações, backup/restauração, recuperação após parada abrupta, Garnet TLS/ACL/AOF, HTTPS, login, reparo, remoção conservadora e registro/desregistro do serviço ERP.
- A instalação completa passou em runner descartável: assinatura temporária de CI, banco/segredos protegidos, HTTPS, reparo, desinstalação preservando PostgreSQL/Garnet/dados e simulações de update/rollback sem alterar SQL. No teste de navegador, **6 passaram e 4 foram ignorados** em cada laboratório; as rotas foram abertas em Chromium/Firefox e a matriz móvel não teve overflow horizontal.
- Artefatos: [prévia técnica unsigned](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41288736458/artifacts/13076705498), [laboratório integrado](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41288736458/artifacts/13076443986) e [primeira instalação/reparo/remoção](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41288736458/artifacts/13076184494).

### Auditoria final

- As **22 issues mapeadas** neste relatório foram conferidas ao vivo e continuam abertas; nenhuma foi fechada automaticamente. A matriz de testes das **19 áreas** permanece acima.
- #160: o isolamento corrigido foi executado duas vezes no mesmo SHA em bancos independentes e passou; a issue fica aberta para revisão do responsável e análise da falha histórica.
- #26: marca configurável e recibo claramente não fiscal implementados. Nome público definitivo, aprovação do proprietário e validação jurídica permanecem pendentes.
- A cobertura automatizada não equivale a aceite visual humano de todas as 19 áreas. O runner registrou screenshots e abriu as 17 rotas principais em Chromium e Firefox; a revisão visual completa ainda precisa de operador/proprietário.

### Limites antes de produção

O Setup permanece uma prévia técnica sem assinatura comercial. O ensaio Windows usou runner e credenciais sintéticas; não simulou aceite de cliente. O clique interativo em Apply, reboot integral de um Windows limpo, dois PCs na LAN, restauração de share externo, periféricos, aceite contábil/operacional, Authenticode/proveniência comercial e aprovação jurídica da marca ainda precisam de execução externa. Em fiscal, o runner desligou transmissão SEFAZ e usou provedor XML local: certificado/credenciamento genuínos, homologação, autorização real e `nfeProc` autorizado continuam bloqueando produção. Pix/TEF reais e seus contratos/credenciais também não foram usados.

Nenhum merge, release, instalação em cliente ou transmissão fiscal foi feito. A cópia de trabalho `D:\ERP` permaneceu intacta; o código e este relatório foram atualizados somente na branch remota da PR #216.
## Atualização 04/11/2026 — cobertura estrutural de 19 áreas

- Revisão estática conferiu 17 rotas da aplicação: 17/17 mapeadas para pastas de evidência. A enumeração das rotas e dos fluxos encontrou IDs de área de 01 a 19, sem lacunas.
- O smoke local agora inclui captura de quantidade inválida no PDV e de acesso de Caixa negado a Relatórios. O caso de acesso nega o conteúdo dos relatórios e captura a mensagem genérica; venda aprovada também mantém captura separada.
- Verificações do arquivo do smoke passaram: ESLint focado, compilação TypeScript isolada, git diff --check e descoberta Playwright com 10 casos em 3 projetos.
- A descoberta Playwright apenas lista os testes. Nenhum E2E funcional foi executado e nenhuma imagem das telas foi gerada: Windows convidado ainda trava no EFI antes do Setup.
- web/smoke/installed-lab.spec.ts permanece modificado no clone local da branch codex/integration-212-215-20261101, sem commit ou push. PR #216 segue aberta, Draft e mergeable no head a02551f4b34a38afba6d8e840f48aa4912615a3b. Sem merge ou release.

## Revalidação da VM após inspeção do VirtualBox (04/11/2026 15:32 -03:00)

- A interface do VirtualBox e a janela `Esterium-Windows-Lab` estão abertas; o Gerenciador responde. A VM aparece como `Executando`.
- A árvore do VirtualBox mantém o snapshot `Base vazia antes do Windows - 2026-11-03`; estado atual aparece como modificado. Snapshot não foi restaurado nem removido.
- Tela convidada continua preta. A barra de status confirma 1 disco rígido, 1 unidade óptica com 1 imagem montada e 4 processadores; o motor mostra execução nativa, paginação aninhada inativa, execução irrestrita inativa e interface de paravirtualização Hyper-V.
- Leitura da configuração `.vbox` confirma firmware EFI, 4 vCPUs, disco VDI e ISO oficial anexada. O caminho da mídia na configuração aponta para Downloads; a cópia preservada e seu SHA-256 constam acima.
- `VBox.log` permanece sem avanço desde 14:56:18 BRT; última entrada continua em `PciHostBridgeDxe.efi` (cerca de 36 minutos antes da observação). A VM não chegou ao Windows Setup.
- A enumeração por PowerShell/VBoxManage neste contexto não conseguiu listar processos/VMs (`E_ACCESSDENIED` em `VBoxManage`), enquanto a inspeção direta da interface confirmou Manager e VM ativos. Não foi feita elevação de privilégio nem mudança de configuração para contornar isso.
- Após o usuário informar que não pode liberar o computador agora, nenhuma nova interação com a interface foi feita. Não houve encerramento/reinício da VM, mudança no Windows principal, instalação, aceite de termos, conta ou credenciais.

### Retomada segura

Quando o computador puder ser liberado, retomar pela tela da VM e revisar, em leitura, EFI, boot e aceleração. A atualização oficial do VirtualBox 7.2.8 para 7.2.22 pode ser avaliada; exige parada controlada da VM e atualização no host, depende de autorização específica e não garante resolver o travamento. Manter Hyper-V e Core Isolation/Memory Integrity ativos. Se a VM continuar preta, interromper testes reais e providenciar outro host Windows compatível.

## Verificação somente leitura da ISO e do changelog (04/11/2026)

- A listagem direta da ISO oficial confirmou `bootmgr.efi`, `efi\boot\bootx64.efi`, `sources\install.wim` e `sources\setup.exe`. A ISO não foi extraída nem modificada. Isso confirma os arquivos de boot e Setup esperados na mídia; não prova que o firmware da VM os esteja lendo.
- O manual oficial 7.2.22 classifica a versão como manutenção com correções de segurança. O changelog consultado não lista correção específica para a parada EFI em `PciHostBridgeDxe.efi` ou para este caso NEM. Entre 7.2.8 e 7.2.22 há correções próximas, mas distintas: 7.2.14 ajusta uso de XCR0 com Hyper-V e um reboot do Windows 11 durante atualização de DBX; 7.2.10 trata boot EFI ARM com menos de 1024 MiB. Nenhuma descreve o sintoma atual.
- Fontes oficiais: [índice do VirtualBox 7.2.22](https://download.virtualbox.org/virtualbox/7.2.22/) e [manual 7.2.22, changelog](https://download.virtualbox.org/virtualbox/7.2.22/UserManual.pdf). Atualizar o host segue como tentativa possível, sem garantia de correção e com parada da VM; manter as proteções do Windows.

## Revisão final do smoke local (04/11/2026)

- `installed-lab.spec.ts` registra capturas das 17 rotas e executa, pela interface, ativação sintética de Caixa, negação de Financeiro/Relatórios, erro de quantidade no PDV, suprimento, sangria, venda não fiscal e fechamento conciliado.
- A limpeza da sessão de caixa acompanha o último saldo confirmado pelo servidor; após fechamento aceito, não tenta fechar a mesma sessão novamente.
- ESLint focado, TypeScript isolado, `git diff --check` e Playwright `--list` passaram; a lista contém 10 casos em Chromium, Firefox e Chromium mobile. `--list` só descobre testes; nenhum E2E funcional foi executado.
- Nenhuma tela do produto foi capturada na VM local. O Windows convidado continua bloqueado antes do Setup em EFI; as capturas locais serão produzidas somente após boot real.

## Revalidação Windows descartável (04/11/2026)

- No commit `2167e5410b3631717aa98969ab0ceb0523e512a2`, o CI `41891195256` concluiu com **20/20 jobs aprovados**.
- O Windows Setup `41891195251` concluiu com falha nos jobs de instalação completa e pacote integrado. Ambos reprovaram o mesmo passo do smoke: `selectOption` não encontrou `getByLabel('Função inicial', { exact: true })` em Chromium e Chromium mobile; a tela e o campo existem, e os inputs anteriores foram preenchidos. As capturas das 17 rotas e da execução integrada foram preservadas nos artefatos [instalação completa](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41891195251/artifacts/13354607840) e [pacote integrado](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41891195251/artifacts/13353729406).
- O head `9b8489a15f272f59501591e53834adc7143a6700` corrigiu esse primeiro seletor. No Windows Setup `41895581279`, o fluxo chegou à abertura do caixa, mas expirou no seletor exato `getByLabel('Produto', { exact: true })`; a limpeza via página fechada deixou uma sessão aberta, contaminando os testes seguintes. As capturas desse avanço estão nos artefatos [instalação completa](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41895581279/artifacts/13355758013) e [pacote integrado](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41895581279/artifacts/13355913361). O CI `41895581296` teve 19 jobs aprovados e um E2E cancelado; não é gate completo.
- Esta revisão usa correspondência por prefixo para o seletor `Produto`, confirma sua visibilidade e limpa a sessão aberta por API independente da página. ESLint focado, TypeScript isolado, descoberta Playwright (10 casos em 3 projetos) e `git diff --check` passaram; o fluxo funcional deste ajuste ainda aguarda CI.
- O resultado do CI não homologa a VM local: ela segue bloqueada no EFI, sem instalação GUI, reboot, backup/restauração, rede ou segurança comprovados no convidado. Sem merge ou release.

## Continuação remota enquanto o PC está indisponível (04/11/2026)

- O usuário informou que não pode liberar o computador agora. Nenhuma nova interação com VirtualBox ou com o Windows principal foi feita; o estado atual da VM não foi confirmado. A última evidência disponível é a tela preta presa em `PciHostBridgeDxe.efi` e o log da VM sem avanço desde 14:56 BRT.
- A PR #216 segue aberta e Draft no head `f4a695d8e1133ed2b43e2b19694a475d5ba85e9a`. O CI [41900274509](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41900274509) passou em **20/20 jobs**.
- O Windows Setup [41900274502](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41900274502) falhou nos jobs de instalação completa e pacote integrado. Os smokes de navegador terminaram com erro de certificado autoassinado em `APIRequestContext.get` ao consultar a sessão de caixa durante a limpeza. Como a falha ocorreu em `finally`, o resultado não confirma se os passos anteriores de venda e fechamento passaram; é necessário repetir o fluxo após corrigir a limpeza. Evidências disponíveis: [primeira instalação](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41900274502/artifacts/13359127160) e [pacote integrado](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41900274502/artifacts/13358407963).
- A correção local troca a requisição Node por uma página Chromium nova e mantém a validação HTTPS habilitada. ESLint focado, compilação TypeScript isolada, descoberta Playwright (**10 casos em 3 projetos**) e `git diff --check` passaram. O fluxo funcional dessa correção ainda não foi executado nem enviado ao CI.
- A homologação que exige iniciar o Windows convidado, instalar pela interface, reiniciar o sistema e registrar backup/restauração, rede e segurança segue pendente até o PC estar disponível e a VM inicializar. Sem merge ou release.
