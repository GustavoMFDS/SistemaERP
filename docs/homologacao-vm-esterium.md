# Homologação do Esterium ERP em VM Windows

**Atualizado em:** 07/11/2026, 02:50 BRT
**Resultado geral:** **EM ANDAMENTO — CI verde no head testado; Verify/Plan confirmados sem alterações; mídia VM-LAB preparada; instalação GUI e 19 áreas pendentes**

## Histórico da fase inicial (04/11/2026, 21:47 BRT)

A VM original continua salva; seu log anterior registra `EFI: Boot failure`/`VMBootFail`. Após a atualização autorizada do host para VirtualBox `7.2.22r175245`, iniciei a VM limpa de diagnóstico e reiniciei apenas o hardware virtual. Depois da falha EFI inicial, a instalação do Windows avançou. O usuário informou **77%**; OOBE chegou à tela de conta de trabalho/escola. Tecla Tab em 21:35 só mostrou validação de e-mail vazio; Enter em 21:37 não abriu opções. Captura às **21:47** confirma a mesma tela. Nenhuma credencial foi inserida. O desktop ainda não foi alcançado e nenhum teste funcional ou visual do Esterium foi executado.

A VM original segue salva. Consultas elevadas e somente leitura até **21:47 BRT** confirmaram a VM limpa **em execução**, VirtualBox `7.2.22`, Secure Boot ativo, ISO oficial anexada e snapshot-base `Blank baseline before Windows - 2026-11-04` (`0f4c1246-b139-44bd-9f94-8ad5bb1c848a`) preservado. A captura daquele momento mostrava “Let's set things up for your work or school”. Um Tab provocou somente validação do campo vazio; Enter não abriu o menu. Nenhuma credencial foi digitada, nenhuma configuração ou chave NVRAM foi alterada. Capturas de OOBE: `D:\ERP\evidencias\esterium-vm\instalacao\windows-setup-check-20261104T2130.png`, `D:\ERP\evidencias\esterium-vm\instalacao\oobe-tab-20261104T2135.png`, `D:\ERP\evidencias\esterium-vm\instalacao\oobe-tab2-20261104T2136.png`, `D:\ERP\evidencias\esterium-vm\instalacao\oobe-signin-options-20261104T2137.png` e `D:\ERP\evidencias\esterium-vm\instalacao\oobe-current-20261104T2147.png`.

A captura das 21:47 ainda não mostrava o desktop; Windows e OOBE foram concluídos depois, conforme as evidências cronológicas abaixo. O usuário aceitou o contrato do Oracle para atualizar o VirtualBox do host. Não houve uso de credenciais nem transmissão fiscal.

## Ambiente identificado

| Item | Estado observado |
|---|---|
| VM original | `Esterium-Windows-Lab`, UUID `866ffc89-9fa4-41de-bc3b-dad41fa29ae7`; estado salvo, snapshot preservado |
| VM limpa de diagnóstico | `Esterium-Windows-Lab-Fresh-20261104`, UUID `b9774f34-583e-491e-9b2e-d11702f59476`; `running` às 00:45 BRT; desktop alcançado, tela atual preta após troca de ISO; VDI próprio de 80 GB e snapshot-base preservado |
| Host | Windows 11, build `10.0.26200.9711` |
| VirtualBox instalado | `7.2.22r175245`; atualizada pelo instalador Oracle oficial, com assinatura Authenticode válida; atualização verificada após saída `0` |
| Sistema convidado | Windows 11 x64; desktop confirmado após OOBE; testes do Esterium ainda não executados |
| CPU e memória | 4 vCPUs, 8 GiB RAM |
| Disco | VDI dinâmico de 80 GB, com VDI diferencial da sessão atual |
| Firmware e segurança | EFI e Secure Boot ativos; TPM 2.0 não confirmado na configuração efetiva |
| Rede | NAT; nenhum teste de acesso LAN executado |
| Mídia | ISO técnica UDF 1.02 da build corrigida anexada; ISO VM-LAB nova do run `42340706284` preparada localmente, ainda não anexada; ISO oficial foi usada para concluir o Windows |
| Snapshot | `Base vazia antes do Windows - 2026-11-03`, UUID `fde9f7b6-c88c-49da-a148-cdc691bd54a7` |

A descrição textual antiga do snapshot menciona 1 vCPU; a configuração ativa e a barra de estado mostram 4 vCPUs. A divergência documental não foi alterada.

Inspeção dos `.vbox` das duas VMs não encontrou configuração TPM; `showvminfo` também não lista TPM. A tentativa de `VBoxManage modifyvm --tpm-type=2.0` retornou sucesso, mas não gravou a configuração. Não contornei esse requisito do Windows 11. O cadastro das chaves padrão de Secure Boot na VM nova falhou porque a sessão estava travada; nenhuma chave foi alterada. O envio de F12 pela interface expirou.

O Gerenciador também lista `Treino` como **Inacessível**. O registro global aponta para `C:\Users\gusta\VirtualBox VMs\Treino\Treino.vbox`, mas nem o arquivo nem a pasta existem. Não foi possível identificar seu sistema, hardware, discos ou snapshots; a entrada foi preservada sem remoção ou re-registro. `Get-VM` não listou máquinas Hyper-V. `VBoxManage` sem elevação retorna `E_ACCESSDENIED`; consultas elevadas foram somente leitura.

## Código e pacote

| Item | Resultado |
|---|---|
| Branch | `codex/integration-212-215-20261101` |
| Head documental local/remoto | `b6892a1e8af5f3ac9a86b230e90bc81a83a3ba7c` |
| Código aprovado em CI | `ac83fe5b7e21ffaf1043f25a7a6a66de2c8c3a50` |
| PR #216 | Aberta, Draft, sem merge; head `b6892a1e8af5f3ac9a86b230e90bc81a83a3ba7c` |
| CI do head atual da PR | [42340706286](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42340706286), 20/20 jobs concluídos com sucesso no head `b6892a1e8af5f3ac9a86b230e90bc81a83a3ba7c` |
| Windows Setup em runner descartável | [42340706284](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42340706284), cinco jobs concluídos com sucesso (prévia, integração ZIP, primeira instalação EDB/ZIP e gate agregado) e quatro jobs manuais ignorados. Isso não é prova da VM local |
| Prévia técnica do Setup da CI | ZIP local conferido: SHA-256 `315ABD762764CD4F14F208FB2EC972CA273B997184C78D769EEBB1293A370475`; contém `EsteriumERP-Setup.exe` de 91.136 bytes, sem bundle/fornecedores; botão Instalar desativado no modo preview; não executado |
| Insumos do laboratório GUI | ZIP local SHA-256 `7DECBB77A67B8318BCF4FCFCFEF6BE8023C4F72A04356BAFC71AC58DE6CE9E71`, igual ao sidecar; inclui bundle do commit `a02551f4b34a38afba6d8e840f48aa4912615a3b` e fornecedores verificados; proveniência declara insumos apenas |
| Prévia técnica com payload real, compilada no host | `D:\ERP\evidencias\esterium-vm\instalacao\technical-preview-payload-pr216-20261104\EsteriumERP-Setup-TECHNICAL-PREVIEW.exe`; 463.227.904 bytes; SHA-256 `0095C1C3B66B050A48C39940362D7E7852D4BA91EFBC5B5D7677D2A6FA308011`; inclui bundle e fornecedores fixados; botão Apply continua permanentemente desativado |
| Prévia técnica reconstruída após correção da GUI | `D:\ERP\evidencias\esterium-vm\instalacao\technical-preview-fixed-20261105\EsteriumERP-Setup-TECHNICAL-PREVIEW.exe`; 463.228.416 bytes; SHA-256 `93B72BF4150DF29C3C10B61CA68329F7F97252FFC2BE81BF16C78DB1BCCB63E7`; sem assinatura Authenticode/licença comercial; Apply permanentemente desativado. Código do Setup inclui as mudanças locais ainda não publicadas; bundle embutido é do commit `a02551f4b34a38afba6d8e840f48aa4912615a3b` |
| Inspeção somente leitura do novo EXE | `test-embedded-preview.ps1` confirmou hashes do manifesto, PostgreSQL, Garnet e NSSM; bundle com 379 arquivos. Nenhum fornecedor foi executado |
| ISO UDF da build corrigida | `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-Technical-Preview-Payload-PR216-FIXED-ROOT-20261105.iso`; 464.388.096 bytes; SHA-256 `3102B13FE1759173440AA9EF40EFC88F8464B87B397FEF9D4131ABEDC1437121`; UDF 1.02; `7z t` passou; quatro arquivos na raiz |
| Mídia virtual da prévia anterior | `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-Technical-Preview-Payload-PR216-20261104.iso`; UDF 1.02, 4 arquivos; `7z t` passou; SHA-256 `BA25F9DE1153EE6ADD28D073E45DE2B2418E833507A70E939C9850AB504C4162`; montada após OOBE |
| Smoke checks do executável de prévia, no host | `--self-test`, `--has-payload`, `--has-preview-approvals` retornaram `0`; `--is-retail-build` retornou `3`; todos os gates esperados passaram. Não é execução GUI nem prova na VM |
| Instalador do Esterium aplicado à VM local | Não; nenhuma instalação/serviço foi criada; a tela da VM precisa ser revalidada após a última troca de ISO |
| Setup completo VM-LAB | Artefato `13562460313` baixado e ISO local preparada; não executado na VM. O certificado temporário ainda precisa ser conferido dentro do convidado |
| Atualização VirtualBox no host | Concluída pelo instalador Oracle 7.2.22; saída `0`; versão `7.2.22r175245`. VM limpa em execução; VM original e snapshot-base preservados |

## Verificações locais do checkout (04/11/2026)

- `go test ./...`: todos os pacotes passaram. A primeira execução foi bloqueada pelo sandbox nos testes que abrem servidores em loopback; a repetição local autorizada passou, sem acesso externo.
- `npm run lint`, `npm run build`, `npm run test:assets` e `npm run test:brand`: passaram.
- E2E local sem serviços externos, `catalog-import-parser.spec.ts` e `setup-progress.spec.ts`: 11/11 passaram no Chromium.
- `scripts/verify-esterium-bundle.ps1`: `PASS`, todos os arquivos do bundle conferem com o manifesto SHA-256.
- `git diff --check`: passou. Esses resultados validam código, assets e bundle no checkout; não substituem instalação, interface gráfica nem testes das 19 áreas na VM.

Os resultados de CI comprovam somente os fluxos executados nos runners descartáveis. Não são evidência de instalação ou operação nesta VM.

## Matriz das 19 áreas

| # | Área | Testado na VM | Aprovado | Resultado | Evidência / pendência |
|---:|---|---|---|---|---|
| 1 | Início / Dashboard | Não | Não | **BLOQUEADO** | Windows iniciou, mas o ERP ainda não foi instalado nem iniciado; executar fluxos e capturas desktop/mobile. |
| 2 | PDV | Não | Não | **BLOQUEADO** | Executar venda sintética, dinheiro/troco, disputa e baixa de estoque. |
| 3 | Caixa | Não | Não | **BLOQUEADO** | Abrir, suprir, sangrar, conciliar e fechar caixa. |
| 4 | Produtos | Não | Não | **BLOQUEADO** | Cadastrar, editar, pesquisar e verificar estados vazios/erro. |
| 5 | Estoque | Não | Não | **BLOQUEADO** | Conferir recebimento, baixa, reserva e ausência de saldo negativo. |
| 6 | Compras e Fornecedores | Não | Não | **BLOQUEADO** | Registrar compra sintética e validar recebimento e saldo. |
| 7 | Clientes | Não | Não | **BLOQUEADO** | Criar, consultar e editar cliente sintético. |
| 8 | Financeiro | Não | Não | **BLOQUEADO** | Validar lançamentos, recebíveis, contas a pagar e permissões. |
| 9 | Fiscal | Não | Não | **BLOQUEADO** | Usar somente documentos locais sintéticos, sem transmissão SEFAZ. |
| 10 | Devoluções e Trocas | Não | Não | **BLOQUEADO** | Validar efeitos financeiros e de estoque após devolução. |
| 11 | Funcionários e Permissões | Não | Não | **BLOQUEADO** | Criar perfis sintéticos e comprovar autorização/negação. |
| 12 | Configuração da Loja | Não | Não | **BLOQUEADO** | Validar configuração e isolamento entre empresas/CNPJs. |
| 13 | Movimentações do Estoque | Não | Não | **BLOQUEADO** | Conferir trilha após venda, compra, troca e ajuste. |
| 14 | Histórico de Importações | Não | Não | **BLOQUEADO** | Executar importação local e verificar histórico/erros. |
| 15 | Relatórios | Não | Não | **BLOQUEADO** | Validar relatórios após operações e permissões por perfil. |
| 16 | Login | Não | Não | **BLOQUEADO** | Validar login sintético, sessão e recuperação após reboot. |
| 17 | Ativação de Acesso | Não | Não | **BLOQUEADO** | Validar fluxo e mensagens sem credenciais reais. |
| 18 | Navegação e mensagens por perfil | Não | Não | **BLOQUEADO** | Comparar menus e acessos de perfis sintéticos. |
| 19 | Mensagens, erros e confirmações seguras | Não | Não | **BLOQUEADO** | Produzir erros controlados e capturas das mensagens. |

As capturas da fase inicial mostram o console VirtualBox sem imagem do convidado e o painel de configuração travado. As capturas mais recentes já mostram o desktop Windows e a unidade da prévia; nenhuma registra telas do produto.

## Cobertura automatizada disponível (runner descartável)

- [`installed-lab.spec.ts`](../web/smoke/installed-lab.spec.ts) declara 17 rotas e associa as evidências às áreas `01`–`17`. O teste “installed HTTPS login opens all 17 screens and both route redirects” percorre essas rotas; outros cenários cobrem layout mobile, navegação desktop/mobile, ativação de funcionário, caixa, erros seguros, confirmação de venda e operações sintéticas com concorrência.
- A mesma especificação registra telas transversais da área 18 em `18-navegacao` (lembrete fiscal, navegação/avisos e perfil de caixa) e da área 19 em `19-mensagens` (acesso negado, quantidade inválida e confirmação da venda). Esses fluxos também rodam em desktop e viewport mobile no runner.
- No head atual `5eaebb52218bb24f66b87bce0758ec1fec166332`, o job `complete-first-install-lab` do [Windows Setup run 41913534238](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41913534238) passou em Windows descartável. Artefatos publicados: [browser evidence](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41913534238/artifacts/13365187063) e [Windows lab screens](https://github.com/GustavoMFDS/SistemaERP/actions/runs/41913534238/artifacts/13363824747).
- Esses artefatos e testes de CI não mudam a coluna “Testado na VM”: todas as 19 áreas seguem sem comprovação no host local até executar e revisar evidências dentro da VM Esterium.

## Instalação, recuperação, rede e segurança

| Etapa | Resultado | Evidência / pendência |
|---|---|---|
| Setup.exe pela interface gráfica | **PENDENTE** | A prévia anterior passou por Verify e falhou em Plan por divergência de porta. A correção foi compilada e a ISO nova montada, mas a tela convidada ficou preta; a nova GUI ainda não foi reaberta. Apply segue desativado e não há instalador comercial assinado para instalar o ERP. |
| PostgreSQL, Garnet e serviço ERP | **BLOQUEADO** | Não instalados nesta VM. |
| Reboot completo do Windows | **PARCIAL** | O Windows reiniciou pelo menu Energia em 05/11/2026, 01:10–01:20 BRT, antes da instalação do ERP. Recuperação de serviços e persistência de dados Esterium após instalação continuam pendentes. |
| Backup e restauração PostgreSQL | **BLOQUEADO** | Banco não instalado; sem backup ou restauração local. |
| Recuperação/falha de serviços, reparo, update e rollback | **BLOQUEADO** | Serviços e instalador não foram executados nesta VM. |
| LAN e dois terminais | **BLOQUEADO** | NAT está configurado, mas nenhum cliente ou teste de rede foi executado. |
| Periféricos e impressão | **BLOQUEADO** | Nenhum dispositivo ou fluxo de impressão foi homologado. |
| TLS, ACL, portas, firewall, isolamento e desempenho do instalado | **BLOQUEADO** | A versão instalada não existe neste convidado; não inferir esses resultados da CI. |

## Evidências locais e diagnóstico

- Captura da tela preta do convidado: `D:\ERP\evidencias\esterium-vm\falhas\vm-tela-preta-apos-reinicios-2026-11-04.jpg`.
- Captura do console VirtualBox completo após os reinícios: `D:\ERP\evidencias\esterium-vm\falhas\vm-console-full-apos-reinicios-2026-11-04.jpg`.
- Ao abrir Sistema, o painel de configurações exibiu “Não está respondendo”; a captura dessa tela não foi salva. O processo da VM foi preservado.
- Na retomada após a liberação do PC, em 04/11/2026, o painel ainda mostrava “Não está respondendo”; clicar em Cancelar não o fechou. Nenhum processo foi encerrado.
- A VM nova foi salva em estado antes de qualquer instalação. Log `D:\ERP\evidencias\esterium-vm\Esterium-Windows-Lab-Fresh-20261104\Logs\VBox.log` registra `cdboot.efi`, depois `EFI: Boot failure` e `VMBootFail` aos 35,96 s. A interface não aceitou o envio de F12; Setup não abriu e licença não foi aceita.
- A prévia técnica e o ZIP de insumos estão em `D:\ERP\evidencias\esterium-vm\instalacao\`; a prévia foi baixada da CI e seu hash coincide com o digest publicado. Ela não substitui o Setup completo de laboratório.
- Log: `D:\ERP\evidencias\esterium-vm\Esterium-Windows-Lab\Logs\VBox.log`; inclui `VMBootFail`, `EFI: Boot failure` e reinícios até `PciHostBridgeDxe.efi`.
- `VBoxManage` sem elevação retornou `E_ACCESSDENIED`. Consultas elevadas somente leitura confirmaram ambas as VMs salvas e `Treino` inacessível. `Get-VM` não listou máquinas; o serviço `vmms` não foi encontrado. Nenhuma proteção do host foi alterada.
- Instalador Oracle oficial `D:\ERP\evidencias\esterium-vm\instalacao\VirtualBox-7.2.22-175245-Win.exe`: SHA-256 `AFD0FF1656CBE04ED88CE619594AE021419D97CCE89FA9BDC443E9A963F1713F`, igual ao `SHA256SUMS` oficial; assinatura Authenticode válida, Oracle America, Inc. A tentativa inicial foi cancelada; a atualização oficial foi concluída depois, conforme o checkpoint de retomada em 04/11 abaixo.

## Próximo passo e limites (checkpoint anterior à atualização, 04/11)

Naquele checkpoint, o VirtualBox ainda estava na versão 7.2.8 e a VM não inicializava a ISO oficial. A tentativa cancelada foi seguida por nova execução autorizada; o instalador oficial 7.2.22 foi concluído, conforme registrado abaixo. Proteções do Windows e Hyper-V foram mantidas.

## Retomada observada em 04/11/2026

- A versão do host foi conferida novamente: `VirtualBox.exe` `7.2.8.173730`, registro `7.2.8`, `VBoxManage` `7.2.8r173730`. `VBoxManage list runningvms` sem elevação retornou `E_ACCESSDENIED`; nenhuma alteração foi feita nas VMs.
- Inventário elevado somente leitura confirmou as duas VMs registradas em estado `saved`, sem VM em execução; snapshots `Base vazia antes do Windows - 2026-11-03` e `Blank baseline before Windows - 2026-11-04` permanecem preservados.
- O inventário do Computer Use não conseguiu ler janelas nativas: `Computer Use helper already has an active request`, inclusive após uma tentativa de recuperação do helper. A sessão de espera `92298` encerrou com erro `Start-Process`: “A operação foi cancelada pelo usuário”. A consulta final não encontrou `consent.exe`, instalador Oracle nem `msiexec`; VirtualBox continuou em `7.2.8.173730`. Não determinei se o pedido foi recusado ou fechado; não tentei relançá-lo nem contornar a permissão.
- PR #216 foi verificada novamente: aberta, Draft, sem merge, `mergeable=true`, head `5eaebb52218bb24f66b87bce0758ec1fec166332`.
- Graphify mapeou `web/src`: 48 arquivos de código, ~48.579 palavras, 494 nós, 1.148 arestas e 20 comunidades; corpus só de código, extração AST, custo de tokens 0. Relatório, JSON e HTML estão fora do checkout em `D:\ERP\graphify-websrc-2026-11-04\graphify-out\`. Esse mapa serve à navegação do código e não comprova comportamento funcional nem homologação Windows.
- As 19 áreas funcionais, instalação GUI, reboot, backup/restauração, rede, segurança, desempenho e periféricos seguem sem execução na VM.

Quando o instalador do Windows estiver acessível, a aceitação dos termos do Windows e qualquer credencial devem ser fornecidas pelo usuário. Depois disso, retomar a instalação e executar a matriz; não marcar itens como aprovados antes de obter evidência real na VM.

Continuam externos à homologação local: Authenticode comercial, autorização fiscal/SEFAZ genuína, aceite contábil, Pix/TEF real, periféricos físicos indisponíveis, dois computadores físicos e restauração em infraestrutura externa independente.

## Retomada do OOBE (04/11/2026, 23:15 BRT)

- O usuário informou que não possui e-mail corporativo. Nenhum e-mail ou credencial Microsoft foi usado; o fluxo local do Windows foi escolhido.
- A tentativa inicial criou o nome local `EsteriumLab`; após a VM aparecer desligada, o OOBE reiniciou. A tela rejeitou esse nome como já existente. Para concluir a configuração, foi informado `EsteriumLabResume`; a senha foi deixada vazia por ser usuário local temporário da VM. Ainda não foi confirmado o desktop nem se o perfil está totalmente pronto.
- Região Brasil foi selecionada. Localização e “Find my device” foram definidos como Não. O layout de teclado e demais opções ainda não foram confirmados nesta retomada.
- A VM apareceu `poweroff` durante a revalidação; preservei o disco de trabalho e iniciei a mesma VM normalmente, sem reset, restauração ou alteração do snapshot. Estado atual: `running`, Secure Boot ativo, snapshot-base `Blank baseline before Windows - 2026-11-04` (`0f4c1246-b139-44bd-9f94-8ad5bb1c848a`) intacto.
- O Windows prossegue na atualização do OOBE. Captura às 23:15 BRT mostrou etapa 2/3, download 53%: `D:\ERP\evidencias\esterium-vm\instalacao\oobe-update-progress9-20261104.png`. A configuração ainda não chegou ao desktop.
- O usuário confirmou que não possui `EsteriumERP-Setup.exe` completo assinado. A prévia técnica local mantém **Instalar/Apply desativado**; ela não substitui o pacote comercial e não permite instalação do ERP. Assim, Setup GUI completo, serviços e 19 áreas continuam sem evidência na VM.

## Finalização do Windows Update (04/11/2026, 23:53 BRT)

- O OOBE avançou para atualização oficial do Windows: etapa 2/3 download e etapa 3/3 instalação. A barra atingiu 100%, mas ainda não confirmei conclusão nem desktop; capturas posteriores alternam tela preta e “100% there”. Última captura: `D:\ERP\evidencias\esterium-vm\instalacao\windows-update-post100-3-20261104.png`.
- Em 23:53, `VBoxManage` confirma `VMState=running` e snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a`. Processo da VM responde e `VBox.log` segue recente. Não fiz reset nem restaurei o snapshot.
- O usuário não tem e-mail de trabalho; o caminho local está em uso, sem credenciais Microsoft. A homologação do ERP continua sem começar: pacote `EsteriumERP-Setup.exe` comercial não disponível; prévia técnica mantém Apply desativado.

## OOBE após atualização (05/11/2026, 00:03 BRT)

- Captura das 23:59 BRT de 04/11, `D:\ERP\evidencias\esterium-vm\instalacao\vm-update-progress-20261104-2359.png`, mostra o OOBE na tela “Choose privacy settings for your device”. A conta local `EsteriumLabResume` já foi aceita; nenhum e-mail corporativo nem credencial Microsoft é necessário.
- Na captura, “Location” está em **Yes** e “Find my device” em **No**. Não avancei essa tela nem alterei a preferência de localização. Próxima ação pendente: o usuário escolher a preferência e clicar em “Next”; depois disso, posso retomar a VM.
- Consulta somente leitura às 23:59 BRT confirma `VMState=running`, Secure Boot ativo e snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` preservado. Processo `VirtualBoxVM` respondendo; não houve reset nem restauração.
- Ainda não há desktop nem teste do Esterium. O instalador comercial completo e assinado segue indisponível; a prévia técnica mantém Apply desativado. As 19 áreas e demais cenários permanecem sem homologação local.

## Desktop e prévia montada (05/11/2026, 00:10 BRT)

- O usuário confirmou que avançou a tela de privacidade e chegou à área de trabalho. Captura da área de trabalho: `D:\ERP\evidencias\esterium-vm\instalacao\desktop-confirmed-20261105-0005.png`.
- Após concluir o OOBE, a ISO oficial do Windows foi substituída pela mídia técnica local. ISO montada em `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-Technical-Preview-Payload-PR216-20261104.iso`, SHA-256 `BA25F9DE1153EE6ADD28D073E45DE2B2418E833507A70E939C9850AB504C4162`; unidade convidada `D:` mostra `EsteriumERP-Setup-TECHNICAL-PREVIEW.exe`, licença e avisos de terceiros. Captura: `D:\ERP\evidencias\esterium-vm\instalacao\technical-preview-mounted-20261105.png`.
- Ainda não executei o EXE da prévia na VM. O pacote comercial assinado continua indisponível; a ação de executar a prévia aguarda confirmação do usuário. Nenhuma instalação do ERP foi aplicada.
- Consulta somente leitura às 00:10 BRT confirma VM `running`, Secure Boot ativo e snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` preservado. As 19 áreas funcionais e demais cenários continuam sem evidência local.

## Build técnico corrigido e reteste pendente (05/11/2026, 00:34–00:45 BRT)

- A primeira prévia GUI passou em Verify, mas Plan falhou porque a origem HTTPS padrão usava a porta 443 e o coordenador escutava em `8443`. Captura da falha e do stack trace: `D:\ERP\evidencias\esterium-vm\instalacao\technical-preview-plan-failure-20261105.png`.
- Corrigi a origem inicial para `https://erp.exemplo.com:8443`, passei `-ListenAddress 0.0.0.0:8443` ao coordenador e removi da GUI a saída bruta do PowerShell/exceções. Em falha de Plan, a interface agora deve exibir só a etapa e o código de saída. Ainda falta confirmar esse comportamento na GUI da VM.
- Build local concluído em `D:\ERP\evidencias\esterium-vm\instalacao\technical-preview-fixed-20261105\EsteriumERP-Setup-TECHNICAL-PREVIEW.exe`; 463.228.416 bytes; SHA-256 `93B72BF4150DF29C3C10B61CA68329F7F97252FFC2BE81BF16C78DB1BCCB63E7`. Prévia sem Authenticode comercial; Apply continua permanentemente desativado. O Setup compilado vem do checkout local; ERP e fornecedores embutidos usam os insumos do commit `a02551f4b34a38afba6d8e840f48aa4912615a3b`, portanto este artefato é evidência de laboratório, não release do head da PR.
- `test-embedded-preview.ps1` passou em modo somente leitura: hashes do manifesto, PostgreSQL/EDB, Garnet e NSSM coincidem; 379 arquivos embutidos. Nenhum fornecedor foi executado no host.
- ISO UDF 1.02 nova passou em `7z t`; quatro arquivos na raiz. SHA-256 `3102B13FE1759173440AA9EF40EFC88F8464B87B397FEF9D4131ABEDC1437121`. `VBox.log` registra essa ISO conectada como DVD e transição da VM para `SUSPENDED` e de volta a `RUNNING`; snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` permanece preservado.
- Após a troca de mídia, a tela do convidado ficou preta. Space e tentativas de abrir menus não recuperaram a imagem; a consulta/captura com `VBoxManage` retornou `E_ACCESSDENIED`. Não fechei nem reiniciei a VM, não restaurei snapshot e não preparei/apliquei instalação. Reabrir a nova prévia e repetir Verify/Plan depende de recuperar a tela da VM.
- A build e ISO estão prontas; Setup GUI corrigido ainda não foi validado. PostgreSQL, Garnet e ERP não foram instalados. As 19 áreas, reboot do Windows, backup/restauração, recuperação de serviços, LAN, segurança, desempenho e periféricos seguem sem homologação local.

## Rechecagem da VM e identificação da prévia antiga (05/11/2026, 01:03 BRT)

- A VM está visível como **Executando** no VirtualBox. `VBox.log` foi atualizado às 01:02:55 BRT e registra a ISO fixa `Esterium-Technical-Preview-Payload-PR216-FIXED-ROOT-20261105.iso` no DVD. `VBoxManage showvminfo` continua retornando `E_ACCESSDENIED`; não usei elevação.
- `Esterium-Windows-Lab-Fresh-20261104.vbox` aponta `currentSnapshot` para `0f4c1246-b139-44bd-9f94-8ad5bb1c848a`, mantém a descrição do snapshot `Blank baseline before Windows - 2026-11-04`, e o VDI diferencial `Snapshots/{a4048363-9b63-4d80-964d-6b32b6fba660}.vdi` existe. Não restaurei nem alterei snapshot.
- A área de trabalho voltou. O Explorer mostra `D:` com o EXE do build fixo. A tela do instalador que ainda estava aberta era processo da prévia anterior: usava `https://erp.exemplo.com` sem `:8443` e mostrava novamente a falha antiga de Plan com detalhes da exceção. Fechei somente essa janela; o novo EXE não foi executado.
- Inspeção visual somente leitura de `C:\Program Files` não mostrou `EsteriumOpenSourceStage`. Portanto, não há prova de preparação dos arquivos nesse destino; não afirmo ausência de toda e qualquer mudança fora dele. Nenhum Apply foi executado nesta retomada.
- A nova execução GUI (somente Verificar arquivos e Ver plano) aguarda resposta à confirmação específica já enviada. A captura foi observada pelo Computer Use, mas não foi salva como arquivo local nesta etapa.
- Consulta atual da PR #216 via GitHub: aberta, Draft, `mergeable=true`, sem merge, head `5eaebb52218bb24f66b87bce0758ec1fec166332`. Nenhum estado remoto foi alterado.

## Reinício do Windows ainda pendente (05/11/2026, 01:10 BRT)

- Ctrl+Alt+Del inserido pelo menu do VirtualBox abriu a tela de segurança do Windows; Cancel voltou ao desktop. A interface do convidado respondeu.
- Tentativas de abrir o menu de energia pelo Iniciar e pela tela de segurança, além de Alt+F4, não mostraram opções de reinício. Não houve reboot, reset, desligamento ou restauração.
- Estado observado continua `RUNNING` na janela VirtualBox; `VBox.log` teve gravação às 01:06:32 BRT. O `.vbox` ainda aponta o snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` e a ISO técnica nova. `VBoxManage` sem elevação continua retornando `E_ACCESSDENIED`.
- A execução do novo EXE para Verify/Plan continua aguardando resposta à confirmação específica já enviada. A tela vista nesta tentativa não foi salva como arquivo local.

## Reboot do Windows confirmado (05/11/2026, 01:10–01:20 BRT)

- No menu Iniciar do convidado, selecionei Energia → Reiniciar. O Windows exibiu “Restarting”, depois a inicialização do VirtualBox e “Please wait”, e retornou à área de trabalho. A imagem foi capturada e inspecionada pelo Computer Use nesta sessão; não foi exportada para a pasta local de evidências.
- `D:\ERP\evidencias\esterium-vm\Esterium-Windows-Lab-Fresh-20261104\Logs\VBox.log` foi atualizado às 01:16:39 BRT; registra a transição `RESETTING` → `RUNNING` e, em seguida, `NAT: Link up`. A janela da VM permaneceu **Executando** e o desktop do convidado ficou visível.
- O `.vbox` ainda aponta `currentSnapshot` para `0f4c1246-b139-44bd-9f94-8ad5bb1c848a`; o VDI diferencial segue presente, e a ISO corrigida continua conectada ao DVD. Não restaurei nem removi snapshots.
- Este reboot comprova reinício do Windows convidado. Não comprova recuperação dos serviços Esterium, pois o ERP ainda não foi instalado.
- A prévia corrigida continua sem execução; a confirmação específica para rodar o EXE ainda está pendente. Apply segue bloqueado. Instalação GUI, 19 áreas, backup/restauração, recuperação do ERP, LAN com segundo dispositivo, controles de segurança e periféricos continuam sem prova nesta VM.
- A inspeção de código confirmou que `test-complete-install-lab-ci.ps1` só aceita runner GitHub Actions hospedado e que `test-unsigned-real-plan-ci.ps1` proíbe explicitamente `-Apply` em dispositivo de usuário. Não contornei esses guardas nem alterei os certificados confiáveis do Windows.

## Prévia compilada com bundle do head atual (05/11/2026)

- Compilei Setup e bundle do head local `5eaebb52218bb24f66b87bce0758ec1fec166332`; manifesto SHA-256 `08FAE70D333DA7009779BBE8B20FD7BFDBAC5DEC71E60FEC194FAB8E77754018`, 376 arquivos.
- EXE: `D:\ERP\evidencias\esterium-vm\instalacao\technical-preview-pr216-ui-fix-20261105\EsteriumERP-Setup-TECHNICAL-PREVIEW.exe`; 463.232.512 bytes; SHA-256 `BB093A0AA1FD047DA08A512F40AAC0949BFF3EE9B1B27D0C194071A36BF47FBD`; Authenticode `NotSigned`.
- Artefato sem licença/assinatura comercial. `Apply` fica desativado; portanto, ele serve para Verificar arquivos e Ver plano, não para instalar ERP. Não executei o EXE.
- Ajustei a GUI para encaminhar certificado e chave HTTPS PEM e CIDR privado; porta do listener deriva da origem HTTPS. `git diff --check` passou. Verify/Plan nessa build, instalação, testes das 19 áreas, backup/restauração, rede e recuperação pós-reboot continuam pendentes.

## ISO atual montada para leitura na VM (05/11/2026)

- Criei ISO UDF 1.02 em `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-Technical-Preview-PR216-CURRENT-HEAD-20261105.iso`; 464.388.096 bytes; SHA-256 `9FD0A0245AA2E9541F54B5ED2D7319DD7E2597BDC582078D4F449D99880541CD`. `7z t` passou; 4 arquivos na raiz.
- Montei pela GUI do VirtualBox. `.vbox` aponta o DVD para o UUID `{ec38e138-5830-4924-9126-4856ee5a3526}` e `VBox.log` registra a ISO conectada e a VM retornando a `RUNNING`. Snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` permanece preservado.
- Após a troca de mídia, a imagem ficou preta; clique e Enter trouxeram a área de trabalho. Não executei o EXE. Verify/Plan ainda dependem de confirmação imediata para rodar software novo; nenhuma instalação foi aplicada.

## Checagem da VM após a build (05/11/2026, 02:16 BRT)

- No instante da checagem, a janela `Esterium-Windows-Lab-Fresh-20261104` estava **Executando** e a tela mostrava aviso de encerramento em 10 minutos, com data interna `5 de novembro de 2025 02:16:34`. Não fechei a VM nem tentei cancelar o aviso. A máquina encerrou depois; a retomada está registrada abaixo.
- A tela de status do VirtualBox mostra um disco rígido e uma mídia óptica montada; nenhum dispositivo USB ou pasta compartilhada conectados. O `.vbox` ainda contém o snapshot-base `{0f4c1246-b139-44bd-9f94-8ad5bb1c848a}`, disco diferencial `{a4048363-9b63-4d80-964d-6b32b6fba660}` (31.279.022.080 bytes) e DVD `{ec38e138-5830-4924-9126-4856ee5a3526}` ligado à ISO atual. `VBox.log` foi atualizado às 02:15 BRT e registra a VM em `RUNNING`.
- Consulta somente leitura `VBoxManage showvminfo` retornou `E_ACCESSDENIED (0x80070005)` neste host. Configuração e log foram inspecionados diretamente, sem alteração. O aviso foi observado na tela, mas a captura não foi exportada a arquivo nesta etapa.
- A inspeção do builder confirma que `-VMHomologationBuild` exige modo de release assinado, manifesto aprovado e thumbprint independente. Os quatro binários Esterium (servidor, bootstrap admin, migrations e TLS certgen) estão `NotSigned` nas quatro cópias de bundle locais; logo, nenhuma passa esse gate. `test-complete-install-lab-ci.ps1` também recusa execução fora de runner GitHub hospedado descartável. Não substituí assinatura nem alterei certificados confiáveis.
- O pedido mais recente autorizou a build sem licença; não autoriza executar o EXE. A prévia segue sem assinatura comercial e com `Apply` desativado. Não executei Verify/Plan. Instalação, 19 áreas, backup/recuperação, teste de rede e segurança continuam pendentes.

## VM retomada após encerramento (05/11/2026, 12:50 BRT)

- O processo da VM terminou em `TERMINATED`; reabri o Gerenciador e sua lista mostrou `Treino` como inacessível, `Esterium-Windows-Lab` salvo e `Esterium-Windows-Lab-Fresh-20261104` desligada. Iniciei somente a VM Fresh pela GUI.
- A árvore do Gerenciador continuou mostrando o snapshot `Blank baseline before Windows - 2026-11-04` e `Estado Atual (modificado)`. Não restaurei snapshot. A VM voltou a `RUNNING`, carregou a conta local `EsteriumLab` e chegou à área de trabalho. UEFI tentou primeiro o DVD não iniciável, depois o Windows iniciou do disco.
- `VBox.log` foi atualizado às 12:47 BRT e registra `NAT: Link up`. O `.vbox` continua apontando ao disco diferencial `{a4048363-9b63-4d80-964d-6b32b6fba660}` e à ISO `{ec38e138-5830-4924-9126-4856ee5a3526}`; os indicadores da VM mostram DVD montado, sem USB nem pasta compartilhada. A área de trabalho exibe 05/11/2026 12:49, e o aviso de encerramento não reapareceu.
- A captura foi vista pelo Computer Use, sem exportação local. Não abri o Setup. O pedido atual cobre build sem licença; execução do EXE segue aguardando confirmação específica. As 19 áreas e a instalação funcional continuam pendentes.

## Inspeção estrutural da prévia local (05/11/2026, 12:50 BRT)

- `windows-setup/test-embedded-preview.ps1` passou em modo somente leitura na build atual. Hash do manifesto ERP e hashes de PostgreSQL, Garnet e NSSM coincidiram; payload contém 379 arquivos totais. O EXE e os fornecedores não foram executados.
- Isso comprova integridade do payload embutido, não valida a GUI Verify/Plan, `Apply`, assinatura comercial ou instalação. A execução da prévia na VM continua pendente de confirmação específica.

## Revalidação após build atual (05/11/2026, 13:04 BRT)

- Computer Use lista a VM `Esterium-Windows-Lab-Fresh-20261104` e o Gerenciador VirtualBox em execução; a janela da VM está marcada `[Executando]`. O usuário confirmou que o Windows chegou à área de trabalho. Snapshot-base `0f4c1246-b139-44bd-9f94-8ad5bb1c848a` e ISO técnica atual seguem associados; nenhum snapshot foi restaurado.
- EXE local atual: `D:\ERP\evidencias\esterium-vm\instalacao\technical-preview-pr216-ui-fix-20261105\EsteriumERP-Setup-TECHNICAL-PREVIEW.exe`; 463.232.512 bytes; SHA-256 `BB093A0AA1FD047DA08A512F40AAC0949BFF3EE9B1B27D0C194071A36BF47FBD`; Authenticode `NotSigned`. Build autorizada sem licença concluída. `Apply` permanece desativado.
- A confirmação pontual para abrir esse EXE na VM e executar apenas Verify/Plan ainda não chegou. Não executei o artefato, não instalei serviços nem fiz alterações no convidado após a confirmação de desktop.
- GitHub revalidado: PR #216 aberta, Draft, não merged, head remoto `5eaebb52218bb24f66b87bce0758ec1fec166332`. CI `41913534237` passou 20/20; Windows Setup `41913534238` teve 4 gates aplicáveis aprovados e 4 ignorados. O job de instalação completa ocorreu em runner descartável. Artefatos desse run incluem evidência de navegador, telas do runner e prévia unsigned; nenhum Setup VM-LAB assinado foi publicado.
- As alterações atuais de `EsteriumSetup.cs`, builder e relatórios estão somente no checkout local; não estão no head remoto nem cobertas pelos runs de CI. `git diff --check` passou. A nova GUI ainda não foi validada.
- Todas as 19 áreas continuam sem teste na VM. O reboot do Windows antes da instalação passou; reboot pós-instalação, serviços, dados persistentes, backup/restauração, rede/LAN, segurança da versão instalada e periféricos seguem pendentes.

## Verify e Plan da prévia na VM (05/11/2026, 13:23 BRT)

- Abri pela unidade de DVD `D:` a prévia técnica do head local atual na VM `Esterium-Windows-Lab-Fresh-20261104`. EXE: `D:\ERP\evidencias\esterium-vm\instalacao\technical-preview-pr216-ui-fix-20261105\EsteriumERP-Setup-TECHNICAL-PREVIEW.exe`; SHA-256 `BB093A0AA1FD047DA08A512F40AAC0949BFF3EE9B1B27D0C194071A36BF47FBD`; Authenticode `NotSigned`.
- A ação **Verificar arquivos** concluiu com `PASS`: hashes do ERP e dos fornecedores conferidos com os informados.
- A ação **Ver plano** concluiu com `Plano concluído sem alterações`. A interface mostrou plano técnico não elevado, sem `Apply`: PostgreSQL 16, Garnet e ERP; TLS e banco novos; vendas desativadas.
- Ao navegar pelos botões, uma ação abriu a confirmação separada **Preparar arquivos (não instala)**. Escolhi **Não**; a interface registrou `Preparação cancelada`. Não confirmei essa preparação nem `Apply`.
- A tela foi observada pelo Computer Use, mas não exportada como arquivo de evidência. O snapshot-base permanece preservado; não restaurei nem desliguei a VM nesta etapa.
- A prévia continua sem assinatura comercial e com `Apply` bloqueado. Nenhum ERP/serviço foi instalado. As 19 áreas funcionais, reboot pós-instalação, persistência, backup/restauração, recuperação de serviços, LAN, segurança e periféricos seguem sem homologação real na VM.

## Pré-checagem Windows somente leitura (05/11/2026, 13:31 BRT)

- Revisei o caminho da GUI: a pré-checagem lê estado do Windows, portas, serviços e conta local, sem instalar ou iniciar serviços; exige que ERP e fornecedores já estejam preparados em pastas separadas.
- A tela pediu elevação para o EXE sem assinatura comercial. Como a pré-checagem ainda depende dessas pastas, recusei o UAC; a GUI registrou `UAC cancelado/falhou`. Portanto, a pré-checagem não foi executada e não há resultado de ambiente limpo.
- `Preparar arquivos` e `Preparar dependências` não foram executados nesta etapa. Ambos gravam cópias verificadas em pastas protegidas da VM e exigem UAC. Nenhum `Apply` foi executado.

## ERP preparado; cópia das dependências aguardando UAC (05/11/2026, 13:37 BRT)

- Na GUI, `Preparar arquivos` concluiu com `PASS: ERP verificado preparado em Program Files; NENHUM SERVICO OU BANCO instalado.` A pasta protegida criada no convidado é `C:\Program Files\EsteriumOpenSourceStage`.
- Iniciei `Preparar dependências`; a confirmação informa cópia verificada de PostgreSQL, Garnet e NSSM, sem executar instaladores, criar contas, configurar SQL ou registrar serviços. A confirmação da etapa foi aceita.
- O UAC do convidado solicita elevação do EXE `NotSigned`, origem CD/DVD. As entradas de Computer Use não acionaram o diálogo seguro; a cópia de dependências segue pendente de clique manual em **Yes** pelo usuário. Não tentei contornar o UAC.
- Ainda não há preflight concluído, serviços/banco do Esterium, `Apply` ou instalação funcional. As 19 áreas e os testes pós-instalação seguem abertos. Capturas desta etapa foram observadas, sem exportação local.

## Dependências continuam pendentes após UAC cancelado (05/11/2026, 13:40 BRT)

- Reobservei a mesma VM; a janela segura do UAC já havia fechado. O log do Setup diz `UAC cancelado/falhou: The operation was canceled by the user`; não assumi que a operação foi autorizada nem tentei abri-la de novo.
- O ERP continua preparado em `C:\Program Files\EsteriumOpenSourceStage`. A cópia de PostgreSQL/Garnet/NSSM não terminou; portanto, o preflight que requer os dois estágios não rodou. Nenhum serviço ou banco do Esterium foi criado e `Apply` segue bloqueado.
- A VM continua aberta com o Setup técnico. As 19 áreas e os testes pós-instalação continuam sem evidência local.

## Dependências verificadas e pré-checagem aprovada (05/11/2026, 14:45 BRT)

- Correção do checkpoint das 13:40: após o usuário clicar **Yes** no UAC da VM, `Preparar dependências` concluiu; PostgreSQL, Garnet e NSSM foram copiados e verificados em estágio protegido. Nenhum instalador de fornecedor foi executado.
- `Pré-checar Windows (não instala)` retornou `PASS: arquivos e Windows limpo para futura instalação. Nenhum serviço instalado.` É uma checagem somente leitura. O ERP segue preparado em `C:\Program Files\EsteriumOpenSourceStage`; não há serviço nem banco do Esterium instalado.
- Captura da tela com o resultado: `D:\ERP\evidencias\esterium-vm\instalacao\preflight-windows-pass-20261105.jpg`; SHA-256 `41AE94565285B71351A63FA6114D55B9CB561045FC3CF6ACC61D827EA1412440`.
- O EXE técnico permanece `NotSigned` e `Apply` continua desativado. O run Windows Setup `41913534238` publica evidências de navegador, telas do runner e prévia unsigned; não há artefato Setup VM-LAB assinado. Não contornei o bloqueio de assinatura.
- Continuam pendentes: instalação GUI funcional, homologação das 19 áreas, reboot e recuperação pós-instalação, persistência, backup/restauração, LAN com segundo dispositivo, segurança/periféricos e respectivas evidências. PR #216 segue aberta e Draft; nenhum merge ou release foi feito.

## VM retomada e desktop Windows alcançado (06/11/2026)

- A VM `Esterium-Windows-Lab-Fresh-20261104` voltou a executar e chegou ao desktop do Windows 11 Enterprise Evaluation. A captura está em [`evidencias/esterium-vm/vm-desktop-boot-20261106.jpg`](../evidencias/esterium-vm/vm-desktop-boot-20261106.jpg), SHA-256 `B7C6E48123B241596DA1809EC24121359FB68AB4C99E2114F4E9133B1F67C997`.
- O título da VM mantém o snapshot `Blank baseline before Windows - 2026-11-04`; não restaurei nem removi snapshot. A VM original `Esterium-Windows-Lab` permaneceu salva.
- Esta captura prova somente boot e acesso ao desktop. O registro anterior de 05/11 documenta Verify/Plan da prévia técnica local; não executei novamente o Setup nesta retomada. A automação não conseguiu abrir o Explorador no guest (`SendInput sent 0 of 1 events; GetLastError=87`), portanto nenhuma ação de instalação foi enviada.
- No head documental `437cc468cc84155a5f883fcfab007db757870582`, CI `42284131373` passou 20/20 jobs e Windows Setup `42284131392` concluiu os gates de instalação/integração; quatro jobs exclusivos de `workflow_dispatch` foram ignorados. Essas provas vêm de runners descartáveis, não da VM.
- A instalação funcional, as 19 áreas, reboot pós-instalação, backup/restauração, LAN, segurança e periféricos continuam sem prova local. A prévia unsigned mantém `Apply` bloqueado; PR #216 segue aberta e Draft.

## Verify e Plan da prévia técnica (06/11/2026)

- A captura enviada pelo usuário mostra `Verificar arquivos` com `PASS` (hashes do ERP e fornecedores) e `Ver plano` concluído sem alterações. O plano lista PostgreSQL 16, Garnet e ERP, com TLS e banco novos; vendas permanecem desativadas.
- A versão sem assinatura mantém `Aplicar` bloqueado. Nenhum serviço/banco foi instalado nesta etapa; o resultado comprova apenas integridade e planejamento.
- Evidência arquivada em [`evidencias/esterium-vm/verify-plan-pass-20261106.png`](../evidencias/esterium-vm/verify-plan-pass-20261106.png), SHA-256 `B24C78AC1AFCA10D5A050889E0D30B3601D77F349D276A3B5D19D6C0CCF56929`.
- Permanecem pendentes a instalação funcional, os testes das 19 áreas e as verificações pós-instalação descritas acima.

## Mídia VM-LAB preparada; instalação ainda pendente (07/11/2026)

- Na checagem de 07/11, PR #216 estava aberta e Draft no head testado `cceb6ed57d124d4b4040d430f4294dfaa262a572`. CI `42336526506` concluiu **20/20 jobs com sucesso**. Windows Setup `42336526606` passou todos os jobs aplicáveis, incluindo os laboratórios completos ZIP/EDB; quatro jobs manuais foram ignorados.
- O usuário reenviou a captura de Verify/Plan. SHA-256 `B24C78AC1AFCA10D5A050889E0D30B3601D77F349D276A3B5D19D6C0CCF56929` coincide com a captura arquivada na seção anterior. Ela comprova hashes conferidos e plano sem alterações; não comprova Apply ou instalação.
- Baixei o artefato `esterium-vm-lab-installer` do run `42336526606` e conferi o SHA-256 do ZIP `7FAD424AE6E2F9DA08820C1A3F594EBC4C6889897DB41B769A4F175917685980`. EXE VM-LAB: SHA-256 `1C6057D8CF57EA7F617E614CE6EBECB1DD865FA91DABB4A27597AE4040ADAE96`; publicador temporário `8DDC59B25C86EE45C6FF66014C4E3F009154B409`, validade até `2026-11-21T04:01:47Z`.
- Preparei ISO local `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-VM-LAB-Homologacao-20261107.iso`, 463.755.264 bytes, SHA-256 `9175DF6039F60F0EC77069ED630195DCF442106C50853A80E8E9BA87C7E18C73`. `7z t` validou todos os arquivos; EXE extraído da ISO coincide com o SHA-256 do pacote. A ISO contém EXE, manifestos, certificado público do editor, TLS de laboratório e instruções. O relatório de mídia local é `D:\ERP\evidencias\esterium-vm\instalacao\VM-LAB-MEDIA-RECEIPT-20261107.json`.
- Certificado TLS foi criado em memória, sem importar certificados nem alterar trust stores do host. O par PEM passou pelo mesmo `tls.LoadX509KeyPair` usado no backend e a cadeia/SAN foi verificada para `10.0.2.15`. Certificado de raiz a importar manualmente apenas na VM: thumbprint `9B3E5774EF029944A0C51C311658A65305B9A048`, SHA-256 `B116E932BCB2D7F9DD339DBEBDD539AA0CA9D7CD4435B047B2C98A1463F4804D`. Esse endereço é hipótese da rede NAT padrão e precisa ser confirmado dentro do convidado antes do Apply.
- Leitura local somente leitura do `.vbox` às 01:48 BRT confirmou `currentSnapshot={780d7489-e774-4218-984a-0be11b17aadf}`; snapshot-base `Blank baseline before Windows - 2026-11-04` (`0f4c1246-b139-44bd-9f94-8ad5bb1c848a`) segue registrado. DVD continua na prévia antiga `Esterium-Technical-Preview-PR216-CURRENT-HEAD-20261105.iso` (`ec38e138-5830-4924-9126-4856ee5a3526`); a nova ISO ainda não foi anexada. A consulta de processos não encontrou `VirtualBox`, `VirtualBoxVM` ou `VBoxHeadless`; nenhum convidado estava executando naquele instante. Não alterei a configuração.
- Esta etapa não anexou ISO, iniciou a VM, confiou certificados ou instalou serviços. Quando houver acesso ao PC, revalidar estado da VM; importar as duas raízes de teste nos stores Current User do convidado; confirmar Authenticode `Valid`; então executar Verify, Plan e instalação pela GUI com confirmação manual.
- Após instalação continuam obrigatórios os testes das 19 áreas, reboot e recuperação dos serviços, backup/restauração, rede e segurança. Não há homologação funcional concluída, merge ou release.

## Mídia VM-LAB atualizada; VM aguardando revalidação ao vivo (07/11/2026)

- Reconsulta ao GitHub: PR #216 segue aberta, Draft e sem merge; head `b6892a1e8af5f3ac9a86b230e90bc81a83a3ba7c`. CI `42340706286` passou 20/20. Windows Setup `42340706284` concluiu com sucesso a prévia unsigned, integração ZIP e laboratórios completos EDB/ZIP; quatro jobs exclusivos de `workflow_dispatch` foram ignorados e o gate agregado passou.
- Artefato VM-LAB `13562460313` do Windows Setup tem SHA-256 `949FAF7ECB621966789BEDC864EBB19CEE59DA95377A03C63FF5E4C9CB5DE172`, conferido localmente. EXE: `0A99AEE537688C2A196280D062EC5D1ADE3B5BBE082B983487ABE5102BE9648B`; certificado público bate com o manifesto, thumbprint `7D445C63ABC9001AFF84441670149A5E09CA53B9`, expira em `2026-11-21T04:59:00Z`. No host, Authenticode retorna `UnknownError` porque a raiz temporária não é confiável; não alterei trust stores. A validação final do editor segue para a VM isolada.
- Gerei a mídia local `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-VM-LAB-Homologacao-20261107-r42340706284.iso`, 464.977.920 bytes, SHA-256 `1C7EDE9DBB70DC5106ACDD8E7351C4274E3AA7D6CEE05534B7E32258E7FF8F77`. `7z t` passou; os 23 arquivos extraídos da ISO coincidem por SHA-256 com a pasta de origem; o EXE da ISO bate com o manifesto. Recibo: `D:\ERP\evidencias\esterium-vm\instalacao\VM-LAB-MEDIA-RECEIPT-20261107-r42340706284.json`. A ISO anterior foi preservada. Esta ISO nova não foi anexada à VM.
- Os quatro arquivos TLS são byte idênticos aos da mídia anterior, cujo par PEM passou `tls.LoadX509KeyPair` e cuja cadeia/SAN `10.0.2.15` foi validada. O endereço ainda precisa ser confirmado no convidado.
- Em 07/11 o usuário informou que não pode liberar o PC. A última checagem local registrada, às 01:48 BRT (seção anterior), não encontrou processos do VirtualBox e concluiu que nenhum convidado estava executando naquele instante; essa observação é posterior ao registro de VM `running` às 00:45 com tela preta. O estado atual não foi revalidado ao vivo.
- Permanecem pendentes instalação GUI e validação Authenticode dentro da VM, todas as 19 áreas, reboot pós-instalação, backup/restauração, recuperação de serviços, LAN, segurança/periféricos e respectivas evidências. Sem instalação ou homologação funcional local; PR continua Draft, sem merge ou release.

## Mídia VM-LAB do head c5880bc; preparada offline (07/11/2026)

- Reconsulta ao GitHub confirmou PR #216 aberta, Draft, sem merge, no head `c5880bcc3ab62b8dce7ad7ab3b981a8ab5094552`. O commit de merge sintético `608feac790ea1a01ff0cb0227d33174b83d739f9` tem esse head como parent, junto ao `main` `69d5e205400d3f4b02c35dca9b164bdeaa9cf49c`.
- CI [42344836484](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42344836484) terminou **20/20 jobs com sucesso**. Windows Setup [42344836483](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42344836483) terminou com sucesso: cinco gates aplicáveis passaram e quatro jobs manuais foram ignorados. Isso continua sendo evidência de runners descartáveis, não de instalação na VM local.
- O artefato [esterium-windows-lab-screens](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42344836483/artifacts/13562993150) foi preservado localmente: 76 PNGs, organizados em 19 áreas; cada pasta tem captura Chromium desktop e mobile. O projeto Playwright define mobile como 390×844 CSS px com escala 2. ZIP SHA-256 `97598030874C97A6FA46DFD9C60BB3523EDCF293EB746A3DC3DC093F75D30B4B`; recibo de arquivos e hashes em `D:\ERP\evidencias\esterium-vm\instalacao\WINDOWS-LAB-SCREENS-RECEIPT-42344836483.json`. São capturas de CI, não evidência da VM nem aceite visual manual.
- O artefato `esterium-vm-lab-installer` (`13563453031`) tem ZIP SHA-256 `2E892598CE97AEE82CEDCFC0E140D65BB3D38A4BFF1438CDDA17A9B9CAC21C6E`. EXE `EsteriumERP-Setup-VM-LAB.exe`: SHA-256 `72E32F769C7225ABC46294BD8EC0BA9BEF0C49374B2732A951A45E2E7B207010`; certificado público corresponde ao thumbprint do manifesto `B9118047F572425C38C3853CFCA66AB3D755F28E`, expira em `2026-11-21T05:57:23Z`. No host, Authenticode retorna `UnknownError` com mensagem de raiz não confiável. Não confiei o certificado no host; assinatura temporária serve ao laboratório e não é comercial.
- Preparei a revisão da mídia em `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-VM-LAB-Homologacao-20261107-r42344836483-v2.iso`, 464.781.312 bytes, SHA-256 `C0B546AD858D0C9805391DCFC41B1F5A60D8A3B8A3AE0D17AAB46C3D2D4E08B2`. `7z t` passou; os 12 arquivos extraídos coincidem por SHA-256 com a origem. Recibo: `D:\ERP\evidencias\esterium-vm\instalacao\VM-LAB-MEDIA-RECEIPT-20261107-r42344836483-v2.json`. A mídia sem sufixo `v2` foi preservada, mas está supersedida pela correção das instruções; usar somente a `v2`.
- A cadeia TLS e o par certificado/chave passaram validação local para `10.0.2.15`, `serverAuth` e `tls.LoadX509KeyPair`. Nenhuma trust store do host foi alterada.
- A ISO **não foi anexada nem executada** e nenhuma VM, snapshot ou configuração foi alterada. Antes de instalar, revalidar ao vivo a identidade/estado da VM e o snapshot. A instalação GUI e todos os testes reais descritos nesta missão — 19 áreas, reboot, recuperação, backup/restauração, LAN, segurança e periféricos — continuam pendentes.
- Revisão das capturas suplementares do runner encontrou a exceção `Cannot read properties of null (reading 'filter')` no PDV e em Compras quando o catálogo de produtos está vazio. A raiz era o JSON `null` emitido por uma slice nil no repositório de produtos. O patch da branch garante `[]`, adiciona fallback nas duas telas e inclui regressão E2E com busca sem resultados; ainda aguarda build web e CI, sem validação na VM.

## Retomada de homologação — CI verde, VM aguardando acesso (07/11/2026)

- No head `dadb08278be0a4e059965db7ce2e67de881d43fd`, CI `42349104030` passou 20/20; Windows Setup `42349104037` passou nos gates aplicáveis. E2E EDB e ZIP passaram em runners descartáveis.
- Corrigido o catálogo vazio em PDV/Compras (`null` para `[]`, fallback de tela e regressão E2E). Artefato EDB `13565068655`: 76 imagens em 19 pastas. Revisei só as capturas desktop do PDV e Compras, ambas sem tela de exceção. Isso não comprova a VM.
- ISO v3 preparada offline: `D:\ERP\evidencias\esterium-vm\instalacao\Esterium-VM-LAB-Homologacao-20261107-r42349104037-v3.iso`, SHA-256 `CEE4A093190C020D80F44A4A99E1954B8B286EFB5EEF4CA5BAB81F1A106B8C14`; teste UDF 1.02 e comparação dos 22 arquivos passaram. Não anexada nem executada.
- O inventário de hoje leu três entradas no `VirtualBox.xml` e não encontrou processos VirtualBox/VMware/Hyper-V. Consultas de estado ao VBoxManage e Hyper-V falharam com `E_ACCESSDENIED`/acesso CIM negado; estado de energia não confirmado. O `.vbox` da VM Fresh mantém o snapshot `Before Esterium VM-LAB Apply - 2026-11-07` (`780d7489-e774-4218-984a-0be11b17aadf`) e a ISO técnica antiga. Nenhuma configuração foi alterada.
- O usuário não pode liberar o PC agora. Aguardar acesso antes de qualquer operação GUI. Na retomada, confirmar estado e snapshot no Manager, validar a mídia e o editor dentro do guest; então instalar pela interface e preencher `D:\ERP\evidencias\esterium-vm\instalacao\VM-LAB-CHECKLIST-19-AREAS.md` com evidências observadas no guest.
- Instalação funcional, 19 áreas, reboot e recuperação, backup/restauração, rede e segurança continuam pendentes. PR #216 permanece Draft, sem merge ou release.

## Proteção da VM original — 07/11/2026

- Verificação somente leitura: disco-base, disco do snapshot e NVRAM atual/snapshot (quatro arquivos) mantêm os hashes da cópia de 03/11. Os XML `.vbox`/`.vbox-prev` diferem porque a cópia descrevia a VM vazia antes do Windows.
- O registro da VM original contém estado salvo presente (21.512.767 bytes) e ISO oficial do Windows anexada. Não tocar nessa VM. A VM Fresh é separada e tem seu snapshot próprio; estado de energia segue sem confirmação do hipervisor.
- Nenhum arquivo foi alterado. A mídia v3 continua desanexada.

## Resultado real do Apply e bloqueio encontrado — 07/11/2026

- Correção cronológica: a ISO v3 foi posteriormente anexada à VM Fresh e o usuário autorizou o Apply. A interface retornou `FALHA (código 1)` após mostrar progresso do EDB/PostgreSQL. A evidência de Serviços confirma três entradas Esterium com início **Disabled**, mas não mostra o Status.
- A captura de `C:\ProgramData\Esterium` mostra `DependencyBootstrap`, `GarnetConfig`, `GarnetLogs`, `PostgresApplication`, `PostgresData`, `PostgresInstallOptions`, `PostgresTlsPolicy` e `Secrets`; não mostra `FirstRunReadiness` nem `Installation`. Nenhum conteúdo de segredo foi aberto. As pastas não comprovam banco íntegro ou serviço funcionando.
- Causa provável pela sequência do código: o launcher usava `-NonInteractive` e o bootstrap do primeiro administrador exige entradas `Read-Host`; esse bootstrap ocorre antes do probe e da ativação. É uma inferência alinhada às evidências, não uma mensagem de erro capturada do guest.
- Corrigi o launcher: planos permanecem não interativos e o Apply permite as perguntas do primeiro administrador. Uma asserção de regressão foi incluída no laboratório descartável. O pacote corrigido e o resultado do CI estão registrados abaixo; o reteste na VM continua pendente.
- A VM permanece em estado parcial. Não executar outro Apply nem restaurar o snapshot automaticamente. As 19 áreas e os testes de reboot, recuperação, backup/restauração, LAN e segurança continuam sem aceite no guest. PR #216 continua Draft, sem merge ou release.

## Rebuild corrigido — 07/11/2026

- Commit `bd2ec47735c19c15c5594f0672c78d1c0a874552` atualizado na PR #216. CI [42429216500](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42429216500) passou **20/20**. Windows Setup [42429216310](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42429216310) terminou com sucesso: os laboratórios completos EDB e ZIP, o teste integrado ZIP, a prévia técnica e o gate agregado passaram; quatro gates manuais foram ignorados.
- Artefato [esterium-vm-lab-installer](https://github.com/GustavoMFDS/SistemaERP/actions/runs/42429216310/artifacts/13597526382). EXE SHA-256 `420617B5DE028ED005383BB33B6786014509CEB2BF1938BD097D50C436FA0433`, conferido contra o manifesto. Verificações somente leitura do EXE passaram para self-test, payload, aprovações e identificação VM-LAB; a opção retail foi recusada como esperado. Recibo local: `D:\ERP\evidencias\esterium-vm\instalacao\VM-LAB-ARTIFACT-RECEIPT-20261107-bd2ec47.json`.
- O novo publicador temporário é `8EF25B628F2DFF2AD3D6D3C9772D07E4EAFC6CF2`, válido até 22/11/2026, sem chave privada no pacote. O thumbprint difere do signer anterior já confiado no guest (`6121F879FF600B2930E7764A76D85466621032A5`). Nenhuma trust store do host/guest foi alterada para este novo certificado.
- A VM Fresh segue parcial e não recebeu o novo EXE. Para retestar é preciso um guest limpo ou clone do snapshot pré-Apply e autorização explícita para confiar o novo thumbprint e executar o Apply. Não restaurar snapshot nem repetir a instalação automaticamente. Os testes de 19 áreas, reboot, recuperação, backup/restauração, LAN e segurança seguem pendentes no guest.
