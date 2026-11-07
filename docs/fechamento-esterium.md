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
- A correção `a8614168737f92888295ea7f4b1c7ea2c12a8984` trocou a requisição Node por uma página Chromium nova, mantendo HTTPS verificado. A CI [41904423574](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41904423574) passou em **20/20 jobs**.
- O Windows Setup [41904423578](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41904423578) passou pela limpeza de caixa e revelou outro erro: `getByLabel(/^Produto/)` encontra três elementos (região, select e lista). Os dois smokes falharam nesse seletor; cada execução teve 4 casos aprovados e 4 ignorados. Evidências: [pacote integrado](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41904423578/artifacts/13361515959) e [primeira instalação](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41904423578/artifacts/13360522412).
- O head de código `ac83fe5b7e21ffaf1043f25a7a6a66de2c8c3a50` restringe o seletor ao primeiro combobox da região `Produtos da venda`. A CI [41907778163](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41907778163) passou em **20/20 jobs**.
- O Windows Setup [41907777994](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41907777994) passou nos quatro gates aplicáveis: prévia, pacote integrado, primeira instalação e gate agregado. Quatro jobs reservados a `workflow_dispatch` foram ignorados. Em cada um dos dois laboratórios, o Playwright concluiu **6 casos e ignorou 4**. Artefatos de telas: [pacote integrado](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41907777994/artifacts/13361773772), [primeira instalação, reparo e remoção](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41907777994/artifacts/13363080297) e [prévia técnica](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41907777994/artifacts/13360827555). Esses registros são do runner descartável, não da VM local.
- A PR #216 continua aberta, Draft e sem merge. O computador do usuário segue indisponível; não houve nova interação com a VM ou alteração no Windows principal. A homologação local de instalação pela interface, reboot do sistema, backup/restauração externa, rede e segurança continua pendente.

## Inventário somente leitura da VM (04/11/2026)

- A configuração `Esterium-Windows-Lab.vbox` ainda aponta para o snapshot `Base vazia antes do Windows - 2026-11-03` (`fde9f7b6-c88c-49da-a148-cdc691bd54a7`), EFI, Windows 11 x64, 4 CPUs, 8 GiB de RAM e TPM 2.0. A ISO oficial `26300.9457.260913-1737.26h2_ge_release_svc_refresh_CLIENTENTERPRISEEVAL_OEMRET_x64FRE_en-us.iso` continua anexada e o arquivo existe.
- O `.vbox` e o VDI diferencial do snapshot seguem com última gravação em 14:49 BRT; o disco-base não teve gravação desde 03/11. `VBox.log` continua com última gravação em 14:56 BRT e a última entrada é `PciHostBridgeDxe.efi`.
- A consulta somente leitura `VBoxManage list vms` falhou com `E_ACCESSDENIED`; a enumeração Hyper-V também falhou. Nenhum processo VirtualBox/VMware/QEMU ficou visível nesta sessão e não há `vmrun` ou QEMU nos caminhos padrão verificados. Isso não confirma se a VM está ligada em outra sessão; o estado permanece desconhecido.
- Nenhum comando iniciou/parou a VM, mudou configuração, restaurou snapshot, alterou o hipervisor ou o Windows principal.

## Comparação das cópias e inventário do hipervisor (04/11/2026)

- O executável do VirtualBox informa `7.2.8r173730`. `VBoxManage list vms` continua retornando `E_ACCESSDENIED`; a consulta Hyper-V falha e o inventário WMI de processos não está disponível. A busca limitada às pastas comuns encontrou apenas a VM do Esterium e suas duas cópias (`baseline-empty` e `with-snapshot`), sem outra configuração `.vbox`/`.vmx` candidata nesses locais.
- A cópia `baseline-empty` não tem snapshot nem ISO anexada; `with-snapshot` contém o snapshot vazio original. As duas são backups da mesma VM, não um Windows já instalado e pronto para homologação.
- SHA-256 confirmou que o VDI-base atual corresponde ao backup `baseline-empty` e que a NVRAM do snapshot corresponde ao backup `with-snapshot`. Não foi possível calcular o hash do VDI diferencial do snapshot: o Windows reportou que outro processo mantém o arquivo aberto. Nenhum processo VirtualBox apareceu na enumeração local, portanto esse bloqueio não prova que a VM esteja executando; o estado permanece desconhecido. O VDI não foi reaberto nem alterado.
- Não atualizei o VirtualBox 7.2.8 nem mexi nas cópias. Sem acesso ao PC e sem identificar quem mantém o VDI aberto, iniciar, parar, restaurar ou duplicar a VM não é seguro nesta sessão.

## Retomada local com o PC liberado (04/11/2026)

- A janela `Esterium-Windows-Lab` estava ativa. O convidado continua sem imagem do Windows e não chegou ao Setup. A ISO Enterprise Evaluation oficial permanece montada; os arquivos EFI/Setup da ISO já tinham sido verificados em leitura.
- O log registra `EFI: Boot failure` aos 29 s e mensagem `VMBootFail`. Dois reinícios controlados repetiram a inicialização até `PciHostBridgeDxe.efi`; a tela continuou preta. `VBox.log` foi atualizado até 18:28 BRT e termina nesse ponto EFI.
- A configuração continua referenciando o snapshot `Base vazia antes do Windows - 2026-11-03` (`fde9f7b6-c88c-49da-a148-cdc691bd54a7`), e o VDI diferencial continua presente. Não restaurei nem removi snapshot, não alterei `.vbox`, mídia, firmware, hardware ou rede.
- `Esc` e `F12` não abriram o menu EFI. O painel de configurações do VirtualBox ficou em “Não está respondendo” ao abrir Sistema. Não encerrei o processo da VM; nenhuma alteração foi feita no Windows principal.
- `VBoxManage showvminfo` retorna `E_ACCESSDENIED`. O cmdlet `Get-VM` existe, mas o serviço `vmms` não foi encontrado; consultar o recurso Hyper-V exige elevação. Não habilitei recurso nem contornei a restrição.
- Ao reabrir o Gerenciador, ele mostrou a VM `Treino` como `Inacessível`, além da VM Esterium em execução. `VirtualBox.xml` aponta `Treino.vbox` para `C:\Users\gusta\VirtualBox VMs\Treino\Treino.vbox`, mas o arquivo e a pasta não existem. Sistema, discos e snapshots de `Treino` permanecem desconhecidos; não removi nem re-registrei essa entrada.
- O painel de configurações voltou a travar. O Gerenciador pôde ser reaberto, mas não consegui fechar o painel sem fazer o processo da VM retornar a “Não está respondendo”.
- Capturas locais do console preto: `D:\ERP\evidencias\esterium-vm\falhas\vm-tela-preta-apos-reinicios-2026-11-04.jpg` e `D:\ERP\evidencias\esterium-vm\falhas\vm-console-full-apos-reinicios-2026-11-04.jpg`.
- A PR #216 foi consultada novamente: aberta, Draft, sem merge, `mergeable=true`, head `5eaebb52218bb24f66b87bce0758ec1fec166332`. O código validado em CI permanece `ac83fe5b7e21ffaf1043f25a7a6a66de2c8c3a50`; nenhum instalador foi aplicado à VM. A prévia técnica local e seu hash são descritos na retomada abaixo.
- A homologação real continua bloqueada antes da instalação. As 19 áreas, reboot do Windows convidado, serviços, backup/restauração, LAN, segurança, desempenho e periféricos não foram testados na VM. A evidência de CI citada acima segue sendo de runners Windows descartáveis.

Para retomar no mesmo host, uma atualização oficial do [VirtualBox 7.2.22](https://download.virtualbox.org/virtualbox/7.2.22/) ou outra intervenção administrativa precisaria de autorização específica: altera software do Windows principal, exige elevação e não garante resolver a falha EFI. Manter Hyper-V e as proteções do Windows ativos.

## Retomada após liberação do PC (04/11/2026)

- VirtualBox Manager e VM continuavam abertos. O convidado seguia preto antes do Windows Setup; o painel de configurações mostrava “Não está respondendo”. Clicar em Cancelar não fechou o painel. Preservei o processo da VM, snapshot, ISO e configuração; sem reinício forçado nem alteração no Windows principal.
- Prévia técnica da CI está salva em `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-Windows-Setup-UNSIGNED-TECHNICAL-PREVIEW.zip`. SHA-256 local `315ABD762764CD4F14F208FB2EC972CA273B997184C78D769EEBB1293A370475` coincide com o digest publicado. O ZIP contém EXE preview de 91.136 bytes, sem bundle nem fornecedores, com **Instalar** desativado; não executei.
- Insumos de laboratório GUI estão preparados em `D:\ERP\evidencias\esterium-vm\instalacao\gui-lab-inputs-pr216-a02551f4.zip`. SHA-256 `7DECBB77A67B8318BCF4FCFCFEF6BE8023C4F72A04356BAFC71AC58DE6CE9E71` coincide com o sidecar. Bundle e dependências foram conferidos; o Setup completo ainda depende de identidade temporária criada dentro da VM.
- ISO oficial Windows 11 Enterprise Evaluation permanece local e anexada à VM. Nenhum termo aceito, credencial fornecida, pacote executado, serviço instalado ou teste do Esterium realizado nesta VM.
- Criei VM limpa separada `Esterium-Windows-Lab-Fresh-20261104` (UUID `b9774f34-583e-491e-9b2e-d11702f59476`), VDI próprio de 80 GB e snapshot vazio `Blank baseline before Windows - 2026-11-04`. ISO oficial carregou `cdboot.efi`, mas o log registrou `EFI: Boot failure`/`VMBootFail` aos 35,96 s. Salvei as duas VMs por meio do VirtualBox; sem encerramento forçado.
- Revisão dos `.vbox` confirmou EFI/Secure Boot, mas não encontrou configuração TPM 2.0 em nenhuma VM; descrição antiga do snapshot não comprova TPM. O CLI não persistiu a configuração TPM. Não iniciei a instalação nem contornei o requisito do Windows 11.
- Consultas de inventário do VirtualBox funcionaram em modo elevado e somente leitura: ambas as VMs aparecem `saved`; `Treino` segue inacessível. As tentativas sem elevação continuam retornando `E_ACCESSDENIED`.
- PR #216 segue Draft, sem merge. Hashes e artefatos locais não substituem homologação: 19 áreas e operações Windows reais seguem pendentes até instalar o ERP na VM e executar a matriz.
- A descrição remota da PR #216 ainda contém o checkpoint antigo “PC não pode ser liberado agora”. Não editei o texto público; este relatório local registra a retomada atual.
- O usuário autorizou atualizar VirtualBox no host. Instalador Oracle oficial 7.2.22 está salvo em `D:\ERP\evidencias\esterium-vm\instalacao\VirtualBox-7.2.22-175245-Win.exe`, SHA-256 `AFD0FF1656CBE04ED88CE619594AE021419D97CCE89FA9BDC443E9A963F1713F` e Authenticode válido. Na primeira tentativa, a versão permaneceu 7.2.8; a espera `92298` terminou com “A operação foi cancelada pelo usuário”.
- Nova tentativa autorizada em 04/11/2026: o usuário confirmou o UAC e aceitou o contrato do Oracle; após “Files in Use”, o usuário fechou o Gerenciador e acionou Retry. O instalador terminou com saída `0`; `VBoxManage --version` confirmou `7.2.22r175245`. Consulta elevada e somente leitura confirmou ambas as VMs salvas antes do teste, seus snapshots, EFI e Secure Boot ativos; o host não foi reiniciado.
- Após a atualização, `VBoxManage startvm` iniciou a VM limpa. O primeiro start retomou o estado salvo anterior; um `controlvm reset` reiniciou somente o hardware virtual sem encerrar o processo `VirtualBoxVM`. O log registrou `cdboot.efi` e `EFI: Boot failure`/`VMBootFail` em 2:07; depois disso, a janela avançou para “Installing Windows 11”. A captura mais recente mostra 29%, ainda em andamento. O original continua salvo e o snapshot-base da VM limpa permanece preservado.
- A interface Computer Use falhou ao clicar na VM limpa no Gerenciador (`Input or refresh outcome is unknown; stop and report` após reobservação); usei `VBoxManage` autorizado para iniciar apenas a VM de diagnóstico. A consulta `VBoxManage modifynvram ... listvars` falhou com `VBOX_E_INVALID_OBJECT_STATE` enquanto a sessão gráfica mantém a VM bloqueada; nenhuma chave foi alterada. O usuário informou que o Windows está instalando, então mantive a VM aberta. Na retomada, o estado foi confirmado como `running` e o snapshot-base permaneceu. A última captura direta era 29%; o usuário informou 77% depois. Duas tentativas de ativação com janela atualizada falharam (`failed to activate captured window`), sem nova entrada enviada à interface. Nenhuma tela do Esterium foi homologada ainda.
- No checkout da branch da PR, `go test ./...`, `npm run lint`, `npm run build`, `npm run test:assets`, `npm run test:brand`, verificação do bundle por manifesto e `git diff --check` passaram. A suíte Go precisou de permissão local de loopback; não houve tráfego externo. Isso não cobre execução GUI na VM.
- Em 04/11/2026, GitHub confirmou PR #216 aberta, Draft, sem merge, `mergeable=true`, head `5eaebb52218bb24f66b87bce0758ec1fec166332`. Para esse head, CI [41913534237](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41913534237) concluiu 20/20 jobs com sucesso; Windows Setup [41913534238](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41913534238) concluiu 4 jobs com sucesso e ignorou 4. O job `complete-first-install-lab` passou em runner descartável; os artefatos [browser evidence](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41913534238/artifacts/13365187063) e [Windows lab screens](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41913534238/artifacts/13363824747) não são provas da VM local. Graphify gerou mapa isolado de `web/src` (48 arquivos, 494 nós, 1.148 arestas, 20 comunidades, custo de tokens 0) em `D:\ERP\graphify-websrc-2026-11-04\graphify-out\`; isso não substitui testes de runtime.

## Estado atual da VM de homologação (04/11/2026, 21:47 BRT)

- Consulta elevada somente leitura e captura às 21:47 confirmam a VM `Esterium-Windows-Lab-Fresh-20261104` (`b9774f34-583e-491e-9b2e-d11702f59476`) em `running`, VirtualBox `7.2.22r175245`, EFI/Secure Boot ativos, ISO oficial Enterprise Evaluation anexada e snapshot-base `Blank baseline before Windows - 2026-11-04` (`0f4c1246-b139-44bd-9f94-8ad5bb1c848a`). A VM original permanece salva. Nenhuma configuração ou chave NVRAM foi alterada.
- Na captura das 21:47, o OOBE ainda pedia login de trabalho/escola. Tab só mostrou validação de e-mail vazio; Enter não abriu opções; nenhuma credencial foi digitada. A captura daquele momento não mostra desktop: `D:\ERP\evidencias\esterium-vm\instalacao\oobe-current-20261104T2147.png`.
- O usuário confirmou que não tem `EsteriumERP-Setup.exe` completo assinado. A prévia técnica com payload real foi compilada no host, com Apply permanentemente desativado: 463.227.904 bytes, SHA-256 `0095C1C3B66B050A48C39940362D7E7852D4BA91EFBC5B5D7677D2A6FA308011`. Gates locais passaram (`--self-test`, `--has-payload`, `--has-preview-approvals`, `--is-retail-build=3`). A ISO UDF 1.02 em `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-Technical-Preview-Payload-PR216-20261104.iso` passou `7z t`; SHA-256 `BA25F9DE1153EE6ADD28D073E45DE2B2418E833507A70E939C9850AB504C4162`. Ainda não foi aberta na VM e não habilita instalação.
- O pacote local `gui-lab-inputs-pr216-a02551f4.zip` confere com seu sidecar SHA-256, mas traz bundle, dependências e fontes do Setup, sem EXE completo pré-construído. `windows-setup/test-complete-install-lab-ci.ps1:6-10` restringe Apply a runner Windows hospedado pelo GitHub e assinado com identidade temporária de CI; não vou contornar essa proteção neste host.
- Inventário do run CI `41913534238` retorna só `Esterium-Windows-Setup-UNSIGNED-TECHNICAL-PREVIEW` (36.437 bytes), `esterium-complete-install-browser-evidence` e `esterium-windows-lab-screens`; não há artefato completo assinado para instalar nesta VM.
- O resultado às 21:47 era **incompleto**: desktop ainda não comprovado, Setup do Esterium não executado, sem serviços ou operação do ERP na VM. As 19 áreas, reboot, backup/restauração, rede, segurança, desempenho e periféricos não tinham evidência local. O OOBE de conta local foi concluído mais tarde, conforme as atualizações abaixo.

## Retomada do OOBE (04/11/2026, 23:15 BRT)

- O usuário informou que não tem e-mail de trabalho; o fluxo local foi usado, sem credenciais Microsoft. `EsteriumLab` já constava como nome existente após a VM aparecer desligada. O OOBE aceitou `EsteriumLabResume` e avançou à atualização; senha vazia, sem dados pessoais.
- Região Brasil selecionada; localização e “Find my device” desativados. Layout do teclado e demais escolhas ainda não confirmados.
- Na revalidação, VM apareceu desligada. Reiniciei somente `Esterium-Windows-Lab-Fresh-20261104` pelo disco atual, sem reset/reverter snapshot. Agora `running`, Secure Boot ativo e snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` preservado.
- Captura `D:\ERP\evidencias\esterium-vm\instalacao\oobe-update-progress9-20261104.png` registra Windows Update, etapa 2/3, download 53%. Desktop ainda não apareceu.
- Não há Setup completo assinado disponível. A prévia com payload real continua com Apply desativado; não executei instalação. Homologação real das 19 áreas, reboot, backup/restauração, rede, segurança e periféricos segue pendente.

## Finalização do Windows Update (04/11/2026, 23:53 BRT)

- Windows concluiu as etapas de download e chegou a 100% na instalação, mas ainda não confirmou o fim nem mostrou desktop. Capturas recentes alternam tela preta e “100% there”: `D:\ERP\evidencias\esterium-vm\instalacao\windows-update-post100-3-20261104.png`.
- Às 23:53, VM segue `running`, VirtualBox responde e o log é recente; snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` preservado. Sem reset ou restauração.
- E-mail corporativo não é necessário: OOBE usa conta local sintética e nenhuma credencial real. A homologação do produto segue pendente porque não há instalador comercial assinado; Apply na prévia técnica está desativado.

## OOBE após atualização (05/11/2026, 00:03 BRT)

- A conta local `EsteriumLabResume` foi aceita; não é necessário e-mail de trabalho nem conta Microsoft. Captura das 23:59 BRT de 04/11, `D:\ERP\evidencias\esterium-vm\instalacao\vm-update-progress-20261104-2359.png`, mostra a tela de privacidade do OOBE, ainda sem desktop.
- “Location” está em **Yes** e “Find my device” em **No**. A tela aguarda o usuário escolher a preferência e clicar em “Next”; não alterei essa configuração.
- Consulta somente leitura confirma VM em execução, VirtualBox respondendo e snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` preservado. Sem reset ou restauração.
- Ainda não há Setup completo assinado para aplicar. Portanto, não foram homologadas as 19 áreas, reboot, backup/restauração, rede, segurança, desempenho ou periféricos.

## Desktop e prévia montada (05/11/2026, 00:10 BRT)

- O usuário concluiu a tela de privacidade e confirmou a área de trabalho. Captura: `D:\ERP\evidencias\esterium-vm\instalacao\desktop-confirmed-20261105-0005.png`.
- ISO técnica anexada após o OOBE; SHA-256 `BA25F9DE1153EE6ADD28D073E45DE2B2418E833507A70E939C9850AB504C4162`. O Explorer da VM mostra `EsteriumERP-Setup-TECHNICAL-PREVIEW.exe`, licença e avisos. Captura: `D:\ERP\evidencias\esterium-vm\instalacao\technical-preview-mounted-20261105.png`.
- A prévia ainda não foi executada na VM e nenhuma instalação foi aplicada. Aguarda confirmação do usuário para abrir o EXE. O instalador completo assinado segue indisponível, portanto as 19 áreas e cenários de reboot, backup/restauração, rede, segurança, desempenho e periféricos ainda não foram homologados localmente.
- VM continua `running`; snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` intacto.

## Build técnico corrigido e reteste pendente (05/11/2026, 00:45 BRT)

- A prévia GUI anterior passou em Verify; Plan falhou ao comparar HTTPS na porta padrão 443 com o listener `8443`, e a tela exibiu detalhes locais da exceção. O código local corrigiu a origem padrão, informou `-ListenAddress 0.0.0.0:8443` e passou a registrar apenas etapa/código de saída, sem stdout/stderr bruto.
- Build técnico local: `D:\ERP\evidencias\esterium-vm\instalacao\technical-preview-fixed-20261105\EsteriumERP-Setup-TECHNICAL-PREVIEW.exe`, 463.228.416 bytes, SHA-256 `93B72BF4150DF29C3C10B61CA68329F7F97252FFC2BE81BF16C78DB1BCCB63E7`. Sem assinatura Authenticode comercial; Apply permanece permanentemente desativado. O bundle embutido veio dos insumos do commit `a02551f4b34a38afba6d8e840f48aa4912615a3b`, e o Setup foi compilado do checkout local.
- Inspeção somente leitura confirmou hashes do bundle e dos fornecedores (PostgreSQL/EDB, Garnet, NSSM), total de 379 arquivos; nenhum fornecedor foi executado no host. ISO UDF 1.02 `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-Technical-Preview-Payload-PR216-FIXED-ROOT-20261105.iso` passou `7z t`, tem quatro arquivos na raiz e SHA-256 `3102B13FE1759173440AA9EF40EFC88F8464B87B397FEF9D4131ABEDC1437121`.
- `VBox.log` registra a ISO nova montada como DVD e a VM voltando a `RUNNING`; o snapshot-base segue preservado. A tela do convidado ficou preta após a troca de mídia. Tentei acordá-la com Space, sem efeito; `VBoxManage` no host retornou `E_ACCESSDENIED`. Não encerrei/reiniciei a VM, não restaurei snapshot e não preparei/apliquei instalação. A nova GUI ainda precisa ser aberta para confirmar a correção de Plan.
- Resultado local continua incompleto: ERP, PostgreSQL e Garnet não instalados; nenhuma das 19 áreas funcionais, reboot do Windows, backup/restauração, serviços, LAN, segurança, desempenho ou periféricos foi homologada nesta VM. PR #216 permanece Draft e sem merge.

## Rechecagem local da VM (05/11/2026, 01:03 BRT)

- VM visivelmente em execução, ISO corrigida anexada; snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` continua apontado no `.vbox` e diferencial permanece presente. `VBoxManage` segue bloqueado por `E_ACCESSDENIED` sem elevação.
- A janela GUI restante era da prévia antiga e repetia erro de porta/stack; foi fechada. `C:\Program Files` não mostra `EsteriumOpenSourceStage`. Novo EXE está na unidade ISO `D:` e aguarda confirmação para executar apenas Verify/Plan. Nenhum Apply ou teste funcional das 19 telas foi feito nesta etapa.
- PR #216 reconsultada: aberta, Draft, sem merge, `mergeable=true`, head `5eaebb52218bb24f66b87bce0758ec1fec166332`.

## Reinício do convidado não concluído (05/11/2026, 01:10 BRT)

- Ctrl+Alt+Del abriu a tela de segurança; Cancel retornou ao desktop, confirmando resposta da interface Windows. Os menus de energia e Alt+F4 não abriram opções de reinício. Nenhum reboot/reset/desligamento ocorreu.
- Janela VirtualBox segue `RUNNING`; `VBox.log` atualizado às 01:06:32 BRT. Snapshot-base e ISO técnica continuam referenciados no `.vbox`. Sem elevação, `VBoxManage` segue com `E_ACCESSDENIED`.
- Novo EXE não executado; aguarda confirmação específica. Nenhuma das 19 telas ou reinício foi homologado nesta etapa.

## Atualização da homologação em VM — reboot do Windows (05/11/2026, 01:10–01:20 BRT)

- O Windows reiniciou pelo menu Energia → Reiniciar e voltou à área de trabalho. `VBox.log` da VM Fresh registra `RESETTING` → `RUNNING` e `NAT: Link up`; evidência visual foi inspecionada nesta sessão, sem arquivo de captura exportado.
- Snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a`, VDI diferencial e ISO técnica permanecem referenciados/presentes. Sem restauração de snapshot.
- O reboot do sistema convidado passou; recuperação pós-reboot de serviços Esterium segue sem teste porque ainda não houve instalação do ERP.
- As 19 áreas, instalação GUI, backup/restauração, LAN com segundo dispositivo, segurança e periféricos permanecem abertas. A prévia técnica mantém Apply bloqueado e aguarda confirmação específica para executar. Os scripts de instalação completa disponíveis na árvore são restritos a CI; seus guardas não foram contornados.

## Build da prévia com bundle atual (05/11/2026)

- EXE compilado do checkout local do head `5eaebb52218bb24f66b87bce0758ec1fec166332`; bundle ERP contém 376 arquivos e manifesto SHA-256 `08FAE70D333DA7009779BBE8B20FD7BFDBAC5DEC71E60FEC194FAB8E77754018`.
- Artefato: `D:\ERP\evidencias\esterium-vm\instalacao\technical-preview-pr216-ui-fix-20261105\EsteriumERP-Setup-TECHNICAL-PREVIEW.exe`; 463.232.512 bytes; SHA-256 `BB093A0AA1FD047DA08A512F40AAC0949BFF3EE9B1B27D0C194071A36BF47FBD`; Authenticode `NotSigned`.
- Build incorporou bundle e fornecedores com hashes conferidos. A prévia não tem assinatura comercial e mantém `Apply` permanentemente desativado. Não foi executada no host nem na VM.
- A GUI agora deriva a porta do listener da origem HTTPS informada e encaminha certificado/chave PEM e CIDR privado ao instalador no caminho de Apply. `git diff --check` passou. Ainda falta validar Plan nessa nova GUI; instalação, 19 áreas, backup/restauração, LAN e recuperação de serviços seguem abertas.

## Revalidação da build e do estado da VM (05/11/2026, 13:04 BRT)

- Computer Use lista a VM Fresh e o VirtualBox Manager em execução; usuário confirmou desktop Windows. Snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` e ISO técnica atual seguem preservados. EXE atual tem 463.232.512 bytes, SHA-256 `BB093A0AA1FD047DA08A512F40AAC0949BFF3EE9B1B27D0C194071A36BF47FBD`, Authenticode `NotSigned`; `Apply` desativado.
- PR #216 segue aberta, Draft, não merged, head `5eaebb52218bb24f66b87bce0758ec1fec166332`; CI `41913534237` passou 20/20. Windows Setup `41913534238` teve 4 gates aplicáveis aprovados e 4 ignorados; instalação completa ocorreu em runner descartável, sem Setup VM-LAB assinado publicado.
- Mudanças locais de GUI/builder ainda não estão no PR e não foram cobertas por CI. A confirmação específica para abrir a prévia e executar Verify/Plan segue pendente; não rodei o EXE nem apliquei instalação.
- Correção do status de reboot: Windows reiniciou pelo menu Energia antes da instalação, retornou à área de trabalho. Recuperação de serviços e dados Esterium após instalação segue pendente. Nenhuma das 19 áreas foi testada na VM.

## ISO da prévia atual montada na VM (05/11/2026)

- ISO UDF 1.02: `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-Technical-Preview-PR216-CURRENT-HEAD-20261105.iso`; 464.388.096 bytes; SHA-256 `9FD0A0245AA2E9541F54B5ED2D7319DD7E2597BDC582078D4F449D99880541CD`. `7z t` passou; contém EXE, hash, licença e avisos.
- Montei essa mídia na VM Fresh pela GUI do VirtualBox. O `.vbox` aponta DVD ao UUID `{ec38e138-5830-4924-9126-4856ee5a3526}` e a `VBox.log` registra suspensão/resumo com a ISO nova em `RUNNING`. Snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` continua selecionado.
- A tela ficou preta após a troca; clique e Enter retornaram à área de trabalho. EXE não executado. Verify/Plan aguardam confirmação de ação; nenhuma instalação foi aplicada. As 19 áreas e cenários restantes continuam abertos.

## Estado ao retomar (05/11/2026, 02:16 BRT)

- VM continua em execução, snapshot-base e mídia montada preservados. O convidado exibe encerramento agendado em 10 minutos e data interna de 05/11/2025; não alterei a VM nem cancelei o aviso. `VBoxManage showvminfo` foi negado com `E_ACCESSDENIED`; a configuração `.vbox` e `VBox.log` confirmam o disco diferencial e a ISO atuais.
- O modo `-VMHomologationBuild` exige bundle em modo de release assinado, manifesto aprovado e thumbprint independente. Os quatro executáveis Esterium estão `NotSigned` em todas as quatro cópias de bundle locais; não há bundle elegível para Apply. O instalador completo de CI recusa host fora do runner GitHub descartável. Não contornei esses gates nem alterei certificados confiáveis.
- Prévia do head atual já compilada sem licença/assinatura comercial; tamanho 463.232.512 bytes, SHA-256 `BB093A0AA1FD047DA08A512F40AAC0949BFF3EE9B1B27D0C194071A36BF47FBD`. Pedido do usuário cobre build; não executei o EXE nem Verify/Plan. `Apply` permanece desativado.
- Homologação funcional ainda não concluída: instalação GUI, 19 áreas, backup/restauração, recuperação após reboot, rede/segurança e evidências exportadas continuam pendentes. PR #216 permanece sem merge automático.
- Consulta GitHub ao vivo confirma PR #216 **open**, **Draft**, não merged, head `5eaebb52218bb24f66b87bce0758ec1fec166332`; não houve escrita remota.
- Depois do aviso de encerramento do Windows, a VM foi desligada; reabri o Gerenciador e iniciei somente `Esterium-Windows-Lab-Fresh-20261104`. Snapshot base e estado atual modificado seguem na árvore, sem restore. Windows voltou ao desktop; `VBox.log` registra `NAT: Link up`; ISO continua montada. Setup não executado.
- Revalidação ao vivo das Actions ligadas à PR: run `41907778163` concluiu 20/20 jobs com sucesso; run Windows Setup `41907777994` teve 4 jobs/gates aplicáveis com sucesso e 4 componentes reservados a `workflow_dispatch` ignorados. O laboratório de instalação é runner descartável GitHub. Artefatos listados para esse run são evidência de navegador/telas e prévia unsigned; não há instalador VM-LAB assinado disponível.
- Esses runs validam o código no commit `ac83fe5b7e21ffaf1043f25a7a6a66de2c8c3a50`; a metadata da PR informa que o head remoto atual `5eaebb52218bb24f66b87bce0758ec1fec166332` acrescenta documentação. As mudanças de Setup ainda não commitadas neste checkout local não foram executadas por esses jobs nem pela VM.
- `windows-setup/test-embedded-preview.ps1` passou em modo somente leitura na prévia local: manifesto e hashes de PostgreSQL/Garnet/NSSM conferem, 379 arquivos no payload. Isso não executa nem comprova o fluxo gráfico; a prévia continua sem assinatura e sem `Apply`.

## Atualização 05/11/2026, 13:23 BRT — Verify e Plan em VM

- Na VM `Esterium-Windows-Lab-Fresh-20261104`, abri a prévia técnica atual a partir do DVD `D:`. EXE de 463.232.512 bytes, SHA-256 `BB093A0AA1FD047DA08A512F40AAC0949BFF3EE9B1B27D0C194071A36BF47FBD`, Authenticode `NotSigned`.
- `Verificar arquivos` retornou `PASS` para os hashes ERP e fornecedores. `Ver plano` retornou `Plano concluído sem alterações`, sem elevação e sem `Apply`; o plano listou PostgreSQL 16, Garnet, ERP, TLS e banco novo, com vendas desativadas.
- Recusei a confirmação separada de `Preparar arquivos (não instala)`; a interface registrou `Preparação cancelada`. Não houve confirmação de preparação nem instalação.
- A observação visual não foi exportada para arquivo local. PR/remoto não foram alterados nesta etapa.
- **Fechamento integral ainda pendente:** não há instalação do ERP; as 19 áreas, reboot e recuperação pós-instalação, persistência, backup/restauração, LAN, segurança e periféricos não foram homologados na VM. A prévia unsigned mantém `Apply` bloqueado. Evidência de CI runner descartável não substitui esses testes locais.

## Pré-checagem Windows pendente (05/11/2026, 13:31 BRT)

- A pré-checagem da GUI é somente leitura, mas o código exige antes as pastas de ERP e fornecedores preparadas. Os dois estágios não foram executados.
- O fluxo pediu UAC para o EXE `NotSigned`; recusei a elevação porque a condição de preparo ainda não estava atendida. A interface registrou `UAC cancelado/falhou`; a checagem não rodou e não prova estado limpo da VM.
- Próxima ação de GUI: preparar ERP e dependências em diretórios protegidos da VM; isso copia arquivos verificados, sem iniciar serviço ou executar `Apply`. Aguarda autorização específica, pois grava em `Program Files` e pede UAC.

## Etapa ERP copiada; dependências pendentes (05/11/2026, 13:37 BRT)

- `Preparar arquivos` terminou com `PASS` na GUI. ERP verificado copiado para `C:\Program Files\EsteriumOpenSourceStage`; interface confirma que nenhum serviço ou banco foi instalado.
- `Preparar dependências` foi iniciado. Confirmei a etapa cujo escopo é copiar PostgreSQL/Garnet/NSSM verificados sem executar instaladores ou criar serviços/contas/SQL. A tela agora aguarda UAC para o EXE não assinado.
- A automação não acionou a tela segura do UAC; pedi ao usuário para clicar **Yes** na VM. Dependências ainda não copiadas; sem `Apply`.
- A homologação integral segue em aberto: preflight, instalação, 19 áreas, reboot/recuperação pós-instalação, backup/restauração, rede e segurança ainda sem prova funcional local.

## UAC de dependências cancelado (05/11/2026, 13:40 BRT)

- A reobservação encontrou o Setup novamente visível e sem UAC aberto. O log registra `UAC cancelado/falhou: The operation was canceled by the user`; não repeti a elevação.
- O estágio ERP permanece concluído. PostgreSQL/Garnet/NSSM não foram copiados; não houve preflight, criação de serviços/banco ou `Apply`.
- Sem confirmação de UAC, o próximo estágio permanece pendente. Instalação funcional e matriz das 19 áreas continuam abertas; não exportei captura local.

## Dependências e pré-checagem passaram na VM (05/11/2026, 14:45 BRT)

- Atualização que supersede o checkpoint das 13:40: o usuário confirmou **Yes** no UAC. A GUI concluiu `Preparar dependências` com cópia verificada de PostgreSQL, Garnet e NSSM para o estágio protegido da VM. Nenhum instalador de fornecedor foi executado.
- A ação `Pré-checar Windows (não instala)` retornou `PASS: arquivos e Windows limpo para futura instalação. Nenhum serviço instalado.` O preflight só leu Windows/portas/serviços e os estágios preparados. Nenhum serviço ou banco do Esterium foi criado; `Apply` não foi executado.
- Evidência visual: `D:\ERP\evidencias\esterium-vm\instalacao\preflight-windows-pass-20261105.jpg`, SHA-256 `41AE94565285B71351A63FA6114D55B9CB561045FC3CF6ACC61D827EA1412440`.
- A prévia é `NotSigned` e mantém `Apply` desativado. Reconsulta ao run `41913534238` encontrou somente artefatos de evidência do runner e `Esterium-Windows-Setup-UNSIGNED-TECHNICAL-PREVIEW`; nenhum Setup VM-LAB assinado está disponível. Não alterei confiança de certificados nem contornei o bloqueio.
- Instalação funcional, 19 áreas, reboot/recuperação pós-instalação, persistência, backup/restauração, LAN, segurança e periféricos seguem sem homologação local. PR #216 continua aberta/Draft, head `5eaebb52218bb24f66b87bce0758ec1fec166332`; não houve escrita remota, merge ou release.

## Atualização 05/11/2026 — SignPath OSS

- O usuário escolheu solicitar assinatura gratuita da SignPath Foundation. A inscrição ainda não existe nem foi aprovada. Se aceita, o publicador visível será SignPath Foundation; aprovação manual é exigida para cada assinatura e o aceite não é garantido.
- O builder agora incorpora os hashes de release e o thumbprint aprovado no EXE. A GUI carrega esses valores em campos somente leitura e valida Authenticode antes do `Apply`; o probe `--is-retail-build` também exige payload, metadados válidos e assinatura do publicador aprovado.
- Verificação local: compilação do EXE sem payload, `--self-test`, portas de bloqueio para payload/aprovações ausentes, parser PowerShell e `git diff --check` passaram. Isso não prova um bundle funcional assinado. O probe de instalação completa não foi executado neste host.
- Bloqueios: não há bundle/EXE funcional disponível nesta cópia; o pacote testado por CI usa instalador EDB com redistribuição ainda sob revisão; a rota PostgreSQL ZIP permanece sem `Apply`; não há release funcional pública exigida pela SignPath, conta/configuração/token, MFA e política formal ativa.
- Nenhum pedido foi enviado à SignPath. Não houve publicação, assinatura, release ou merge nesta atualização.

## Bloqueio 1 — rota PostgreSQL ZIP no instalador

- O build assinado agora aceita `-PostgresBinaryZip` com o ZIP oficial PostgreSQL 16.15-5 e SHA-256 fixo. O EXE de release registra `pgFormat=zip`; o parser recusa SHA diferente e thumbprint EDB preenchido.
- O coordenador propaga esse formato por verificação, preparação e instalação. No Apply, valida o ZIP novamente, extrai em estágio protegido, cria a conta local sem admin, concede apenas o direito de logon como serviço, inicializa o cluster com SCRAM e senha temporária ACL-protegida e registra PostgreSQL desabilitado/parado. A etapa TLS existente recebe a mesma prova de cluster novo.
- A CI de instalação completa agora roda em duas VMs Windows descartáveis independentes, uma para EDB e outra para ZIP, usando a mesma aceitação de banco, TLS, ERP, reparo e desinstalação. Essa CI ainda não foi executada para estas alterações; o Apply não foi testado neste PC ou na VM do usuário.
- A página oficial PostgreSQL diz que o ZIP sem instalador é destinado a inclusão em instaladores de outras aplicações; a licença PostgreSQL permite copiar e distribuir sob condições de aviso. O caminho ZIP dispensa redistribuir o EDB EXE. Falta revisar e incluir avisos de eventuais dependências dentro do ZIP antes de publicar: [Windows downloads](https://www.postgresql.org/download/windows/), [PostgreSQL License](https://www.postgresql.org/about/licence/).
- Verificação local: parser PowerShell passou nos oito scripts alterados; `git diff --check` passou; o Setup C# sem payload compilou e o probe confirmou que a porta de instalação segue bloqueada. Um artefato sintético confirmou que o parser aceita metadados `pgFormat=zip` apenas com o SHA fixado e thumbprint EDB vazio. O código LSA compilou em memória, sem invocar a alteração de direitos locais. A pasta temporária de build foi removida.
- Estado: rota implementada, ainda sem homologação E2E. Assinatura SignPath, auditoria dos avisos do ZIP e aceitação do release seguem abertos; nenhuma alteração remota, publicação ou instalação foi feita nesta etapa.

## Retomada da VM e inventário de avisos (06/11/2026, 21:08 BRT)

- Na VM `Esterium-Windows-Lab-Fresh-20261104`, o usuário informou que executou **Verificar arquivos** e **Ver plano**. Observei a prévia técnica aberta, com `Aplicar` bloqueado. O texto de resultado não ficou legível na captura; não registrei novo `PASS` para essas duas ações naquela hora. Não houve `Preparar` nem `Aplicar` nesta etapa.
- O ajuste do inventário exclui `licence.doctree`, cache gerado pelo Sphinx, e valida que caminhos listados coincidam com avisos empacotados. Parser PowerShell, fixture focada e `git diff --check` passaram. O artefato da reexecução [13551295165](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42312733399/artifacts/13551295165) contém 270 caminhos e 270 arquivos, sem `.doctree`; SHA-256 `C817BEA62666CC101A9AD3DE33A8856053EE4F57B1F6BC3DB152421533DA6DD9`.
- CI [42312733314](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42312733314) e Windows Setup [42312733399](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42312733399) passaram. Incluem runners descartáveis; não substituem a instalação/homologação da VM.
- PR #216 segue aberta, Draft e sem merge no head `dd5fecda2a39135f14c0077a19278a5b530c6f04`. Nenhuma release foi publicada.
- Ainda pendem: resultado legível de Verify/Plan na VM, instalação funcional com mídia aprovada, 19 áreas, reboot/recuperação, backup/restauração, rede e periféricos; revisão jurídica das dependências; release funcional e inscrição/aprovação SignPath; política final e MFA.

## Resultado da prévia técnica na VM (06/11/2026, 21:24 BRT)

- A captura enviada pelo usuário mostra **Verificar arquivos: PASS** duas vezes, conferindo hashes do ERP e dos fornecedores; **Ver plano: PASS**, “concluído sem alterações”. O plano lista PostgreSQL 16, Garnet e ERP, com TLS e banco novo; vendas continuam desativadas.
- A tela confirma que a versão sem assinatura permite somente verificação e plano; `Aplicar` segue bloqueado. Esta etapa somente leitura está concluída. Preparar, Aplicar, instalação funcional e homologação completa da VM seguem pendentes.
- Capturei a evidência enviada em [`evidencias/esterium-vm/verify-plan-pass-20261106.png`](../evidencias/esterium-vm/verify-plan-pass-20261106.png), SHA-256 `B24C78AC1AFCA10D5A050889E0D30B3601D77F349D276A3B5D19D6C0CCF56929`. Ela registra os dois `PASS`, o plano sem alterações e o bloqueio de `Apply`.

## Revalidação Windows após a troca para WinSW (07/11/2026)

- Commit `bb75d4f3b3b72fb3eca47258271197aa0c55dce6`: CI [42331313371](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42331313371) passou **20/20**. Windows Setup [42331313342](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42331313342) passou em prévia unsigned, integração ZIP, instalações completas ZIP e EDB e gate agregado; quatro jobs de execução manual foram ignorados.
- A rota ZIP passou a instalação completa depois que a checagem de listener passou a aguardar até 30 segundos por um bind tardio. Em ambos os formatos, o runner descartável passou por criação do banco, TLS, migrações, administrador inicial, HTTPS, reparo e remoção conservadora; testes de atualização/rollback preservaram SQL e dados.
- Integração pelo EXE nativo passou TLS/SCRAM, migrações e recuperação PostgreSQL, Garnet com WinSW/NetworkService, TLS/ACL/Lua/AOF, reinício dos serviços, navegação do ERP e remoção preservando dependências/segredos/dados. A CI usou certificados sintéticos e ambiente descartável; isso não é assinatura pública nem homologação de produção.
- Artefatos: [capturas de tela do laboratório](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42331313342/artifacts/13558012643), [prova completa ZIP](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42331313342/artifacts/13558331449), [prova completa EDB](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42331313342/artifacts/13558247632), [avisos de terceiros ZIP](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42331313342/artifacts/13558026882) e [avisos de terceiros EDB](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42331313342/artifacts/13558102687). Expiração prevista: 14/11/2026.
- A captura da VM do usuário confirma **Verificar arquivos: PASS** e **Ver plano: PASS**, sem alterações; `Aplicar` continua desativado. Não houve instalação funcional nessa VM. Os testes de instalação acima ocorreram somente em runners Windows descartáveis.
- Ainda pendem: assinatura de produção e aprovação SignPath; revisão humana/jurídica dos componentes redistribuídos; release funcional pública e política/MFA; homologação física da VM e do ambiente do usuário, reboot/restore reais, rede/periféricos e aceitação fiscal/comercial com credenciais autorizadas. Vendas/SEFAZ/Pix/TEF continuam desativados. PR #216 permanece aberta, Draft e sem merge.
