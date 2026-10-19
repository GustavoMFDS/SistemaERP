# Registro do serviço Windows — Esterium Local (L2 parcial)

O script `scripts/register-esterium-windows-service.ps1` registra **apenas** o executável Go e os arquivos compilados da interface em `C:\Program Files\EsteriumERP`. Ele **não é um instalador completo** de loja.

**Novo modo LAN:** opcionalmente aceite `-ListenAddress '0.0.0.0:8443' -TLSCertFile 'C:\ProgramData\Esterium\TLS\server-cert.pem' -TLSKeyFile 'C:\ProgramData\Esterium\TLS\server-key.pem'`. Um endereço não-local **sempre exige o par TLS**, guardado fora do ZIP e da pasta do programa, com permissão de leitura explícita para `LocalService`. O Go verifica par/certificado, validade e se o nome do certificado (SAN) corresponde ao `APP_PUBLIC_URL` informado; exige TLS 1.3 e recusa iniciar se houver divergência. Os navegadores dos caixas ainda precisam confiar na cadeia do certificado, e a regra de firewall LAN deve ser configurada independentemente.

O padrão permanece `127.0.0.1:8080` sem TLS, acessível apenas no próprio PC ou por proxy HTTPS externo. O registrador não configura proxy, DNS, firewall ou confiança dos clientes; tampouco substitui a revisão do `APP_PUBLIC_URL` utilizado no acesso real.

## Verificação do computador antes da instalação

O pacote técnico inclui `tools/check-esterium-windows-host.ps1`. Ele confere de forma **somente leitura** arquitetura Windows x64, versão base Windows 10/11, espaço livre mínimo (10 GiB por padrão, não é dimensionamento de banco), se o serviço e a pasta de instalação já existem, uso prévio da porta HTTP e integridade do pacote SHA-256. **Não instala, modifica nem inicia** software, firewall, rede, banco de dados ou backup.

Em uma máquina de testes, com o ZIP técnico extraído:

```powershell
.\tools\check-esterium-windows-host.ps1 -BundleDir (Get-Location).Path
```

Com `-RequireSignedServer`, exige executável com **assinatura Authenticode válida**, rejeitando o bundle técnico atual que ainda não é assinado. Sem essa chave, o script apenas verifica integridade por hash **e avisa explicitamente que não comprovou a origem do pacote**. Um resultado verde não certifica o equipamento, o instalador ou o uso comercial. Ainda faltam validar PostgreSQL e Redis reais, confiabilidade TLS dos clientes, impressoras, restauração externa e SEFAZ.

## Um único comando de planejamento para o técnico

O pacote Windows também contém `tools/plan-esterium-local-setup.ps1`, que **somente consulta** os requisitos para uma **instalação nova**, sem instalar ou iniciar nada. Ele combina três conferências: computador e integridade do pacote, serviços PostgreSQL/Redis compatíveis já existentes e plano do serviço Windows do Esterium. Uma falha interrompe a sequência sem modificar a instalação.

Exemplo de laboratório, após o técnico preparar PostgreSQL/Redis, certificados e arquivos de segredos reais, sem senhas escritas no comando:

```powershell
.\tools\plan-esterium-local-setup.ps1 `
  -BundleDir (Get-Location).Path `
  -PostgresServiceName 'postgresql-x64-16' `
  -RedisServiceName 'EsteriumGarnet' `
  -PostgresBin 'C:\Program Files\PostgreSQL\16\bin' `
  -DatabaseURLFile 'C:\ProgramData\Esterium\Secrets\database-url.secret' `
  -RedisURLFile 'C:\ProgramData\Esterium\Secrets\redis-url.secret' `
  -JWTSecretFile 'C:\ProgramData\Esterium\Secrets\jwt.secret' `
  -MetricsBearerTokenFile 'C:\ProgramData\Esterium\Secrets\metrics.secret' `
  -AllowedOrigin 'https://esterium.loja.example'
```

Esses nomes são **exemplos**, não nomes de serviços automaticamente criados. A ferramenta não prova TLS, versão SQL ou compatibilidade das operações atômicas do Redis; depois dela ainda é obrigatório validar `--check-config` e `--check-dependencies` na identidade real, e realizar os testes do piloto. O parâmetro opcional `-RequireSignedServer` recusa o pacote sem assinatura válida, que ainda é o estado atual do ZIP técnico.

## Confiança do instalador antes de executar PowerShell

Uma assinatura do servidor **não basta** se o técnico executar um script de instalação adulterado. No modo efetivo `-Apply`, `install-esterium-local.ps1` e `apply-esterium-app-update.ps1` agora verificam **antes de chamar qualquer outro script** a assinatura Authenticode da própria entrada PowerShell e do verificador de release, usando a identidade do editor aprovada separadamente. Em seguida, conferem toda a release e o manifesto SHA-256 também aprovado por fonte independente; somente depois podem executar o planejamento ou tentar a troca dos arquivos.

**Proteção necessária fora do script:** antes de iniciar o arquivo, o Windows/operador deve assegurar sua origem e assinatura por um mecanismo confiável (por exemplo, execução restrita a scripts assinados sob política aprovada ou um instalador assinado de distribuição). Um script não confiável já executado **não pode autenticar a si próprio retroativamente**. O pacote técnico não assinado do CI continua bloqueado para implantação real. Os testes validam essa recusa com arquivos sintéticos, não com credenciais ou dados de loja.

## Instalador técnico controlado: planejamento por padrão

O novo utilitário `tools/install-esterium-local.ps1` concentra a preparação e o **registro do serviço do Esterium**. Por padrão ele apenas **simula** a instalação e recusa avançar se a integridade do pacote, a assinatura digital do executável, a existência dos serviços locais PostgreSQL/Redis ou os caminhos dos arquivos protegidos forem inválidos.

Para uma instalação real a opção `-Apply` requer execução administrativa, assinatura Authenticode válida do executável, banco e cache **previamente provisionados**. Ele então chama o registrador seguro já testado para copiar o app para uma pasta nova sob Program Files, conferir SHA-256 e registrar `EsteriumERP` como `LocalService`, com inicialização manual e **sem iniciar o processo**. Essa ferramenta **não executa migrações, não instala PostgreSQL/Garnet, não configura firewall, não gera senhas de banco, não registra nem habilita backup e não ativa SEFAZ**.

O bundle técnico atual **ainda não tem assinatura de distribuição**: por isso, **não será instalado por este comando em máquinas de loja** até o processo de assinatura estar pronto. Existe uma exceção apenas para arquivos sintéticos em CI Windows hospedado descartável (`-UnsignedDisposableCIFixture`), proibida fora desse ambiente. Essa é uma medida de segurança deliberada, e não a conclusão de um instalador `Setup.exe` para usuário final. A instalação automática e confiável das dependências e o assistente visual são marcos posteriores.

## Segurança de instalação

O programa **recusa sobrescrever** a pasta de instalação ou o serviço `EsteriumERP` já existentes. Copia o executável compilado, `web/`, `tools/`, `README_LOCAL.md` e `bundle-manifest.json`; os arquivos são verificados por SHA-256 **novamente na pasta temporária antes do registro SCM**. Nenhum valor de credencial, banco ou backup é copiado. As ferramentas de manutenção ficam disponíveis na pasta instalada, mesmo depois de descartar o ZIP extraído; sua execução e o agendamento de backup continuam procedimentos separados.

O pacote deve incluir `bundle-manifest.json`: todos os arquivos do executável e `web/` são comparados com seus SHA-256 e arquivos extras/links simbólicos são rejeitados **antes da instalação**, inclusive durante `-PlanOnly`. O manifesto é gerado pelo script de build e conferido no CI; ele verifica integridade relativa ao manifesto, **não comprova origem/autenticidade** (assinatura de código ainda pendente).

Se `sc.exe create` der certo, mas a gravação do ambiente no Registro ou a atribuição da identidade de serviço falhar, o registrador tenta **remover somente o serviço criado naquela invocação**. Os arquivos copiados permanecem para inspeção manual; não são apagados dados existentes, credenciais ou outros serviços. O CI Windows injeta uma falha descartável após `sc create`, verifica a reversão e só depois testa o registro normal. Uma falha na própria exclusão gera aviso explícito para intervenção técnica, nunca sucesso.

A instalação efetiva rejeita arquivos de credenciais que sejam links simbólicos, concedam leitura a qualquer identidade fora de **LocalService, SYSTEM e Administradores**, ou permitam escrita pela própria conta LocalService. O CI Windows usa somente credenciais artificiais para exercitar recusas por permissões perigosas, sem abrir banco algum.

A conta padrão do serviço é **LocalService**, sem privilégios administrativos de rotina. O serviço é registrado com inicialização **manual** e só deve ser iniciado depois que os bancos, os certificados TLS de PostgreSQL/Redis e as permissões dos arquivos estejam configurados e as migrações tenham sido aplicadas. O código Go aceita Stop/Shutdown do Gerenciador de Serviços.

### Gerar os segredos que pertencem ao Esterium

O pacote inclui `tools/provision-esterium-app-secrets.ps1`. O modo padrão é uma **simulação sem escrita**; após revisar em um Windows de testes com administrador, execute a variante `-Apply`. Ela cria uma pasta **nova** em `C:\ProgramData\Esterium\Secrets`, gera um `jwt.secret` e um `metrics.secret` independentes (48 bytes aleatórios cada), configura leitura para LocalService e controle para SYSTEM/administradores, sem mostrar os valores no terminal. Se a pasta existir, **não sobrescreve** nada. Gerar novos tokens em uma loja existente revogaria sessões: a rotação requer procedimento controlado.

As credenciais de PostgreSQL e Redis **não são geradas** por esse utilitário: precisam ser provisionadas junto com seus serviços, com conexão TLS verificada, e gravadas em arquivos de URLs separados. O instalador precisa desses quatro caminhos; nenhum deles está incluído no ZIP.

Em ambiente descartável, a partir da pasta extraída do pacote:

```powershell
.\tools\provision-esterium-app-secrets.ps1
# Somente após revisar as permissões, em PC de testes:
.\tools\provision-esterium-app-secrets.ps1 -Apply
```

### Preparar instalação nova do PostgreSQL 16 com arquivo protegido

A ferramenta `tools/provision-esterium-postgres-optionfile.ps1` produz **somente o arquivo de opções de uma instalação PostgreSQL nova**, para o instalador Windows oficial da EDB. Sem `-Apply`, apenas confere caminhos, porta, nomes de serviço/conta e ACL dos dois arquivos de senha, sem gravar nada. Com `-Apply` administrativo, grava `postgresql-unattended.options` num diretório novo sob `ProgramData\Esterium`, legível **apenas por SYSTEM e Administradores**, nunca por operadores ou pelos serviços Garnet/Esterium.

O arquivo declara `mode=unattended`, caminhos novos para programa e dados, porta, conta exclusiva Windows `esterium_pgsvc`, serviço exclusivo `EsteriumPostgreSQL16`, superusuário PostgreSQL inicial, instalação de `server,commandlinetools` e dispensa de pgAdmin/StackBuilder e atalhos. As senhas de superusuário e de serviço são **separadas**, exigem arquivos de origem restritos e ficam somente neste arquivo de opções protegido; nunca são usadas como argumentos de processo, exibidas no terminal ou inseridas no bundle do ERP. Uma instalação antiga, diretório existente, conta/serviço existente ou porta ocupada bloqueia o plano.

Exemplo de preparação **somente leitura**:

```powershell
.\tools\provision-esterium-postgres-optionfile.ps1 -SuperPasswordFile 'C:\ProgramData\Esterium\BootstrapSecrets\pg-super.secret' -ServicePasswordFile 'C:\ProgramData\Esterium\BootstrapSecrets\pg-service.secret'
```

Após aceite técnico, `-Apply` cria o arquivo de opções, mas **não baixa nem executa** o PostgreSQL, não cria cluster, usuário SQL da aplicação, regras `pg_hba.conf`, TLS, banco Esterium nem serviço. Antes de usar o instalador externo, é obrigatório verificar assinatura do fornecedor e seu SHA-256 aprovado independentemente, validar comportamento/versão exata do instalador e garantir um procedimento de descarte seguro do arquivo de opções contendo senhas. Veja a [documentação oficial da EDB](https://www.enterprisedb.com/docs/supported-open-source/postgresql/installing/command_line_parameters/) para `--optionfile` e os parâmetros do instalador. Após a instalação, os testes reais `--check-config` / `--check-dependencies` continuam obrigatórios; não usar o banco de uma loja.

O CI cria esse arquivo exclusivamente com **credenciais sintéticas** em runner Windows hospedado e verifica ausência de operações de banco, não sobrescrita, permissões e recusas a acessos inseguros.

### Preparar o arquivo restrito para o instalador offline PostgreSQL 16

A ferramenta `tools/provision-esterium-postgres-optionfile.ps1` foi projetada **somente para uma instalação nova e ainda vazia**. O padrão é uma análise sem escrita. Com `-Apply` administrativo, cria um arquivo de opções EDB protegido, sob `ProgramData\\Esterium\\PostgresInstallOptions`, para posterior instalação sem interação: configura nome do serviço, conta Windows dedicada, pasta de binários, pasta separada de dados, porta e componentes **servidor + utilitários de linha de comando**, excluindo pgAdmin/StackBuilder. Verifica antes da criação que as pastas de instalação e dados e a conta/serviço alvo ainda não existem, que a porta não está ocupada e que ambos os arquivos de senhas são protegidos contra leitores comuns.

A senha de administrador do PostgreSQL e a senha da conta de serviço são lidas de **dois arquivos separados**, exclusivamente acessíveis por `SYSTEM` e `Administradores`; devem ser diferentes. Seus valores **não são colocados na linha de comando**. O arquivo resultante contém as senhas em texto, pois é assim que a opção `--optionfile` do instalador as recebe, e por isso fica protegido por permissões Windows restritas e deve ser aposentado com segurança após o uso. O comando não imprime nenhum valor.

Exemplo de preparação sem efeito colateral:

```powershell
.\\tools\\provision-esterium-postgres-optionfile.ps1 `
  -SuperPasswordFile 'C:\\ProgramData\\Esterium\\Bootstrap\\pg-super.secret' `
  -ServicePasswordFile 'C:\\ProgramData\\Esterium\\Bootstrap\\pg-service.secret'
```

**Limite:** não executamos `--optionfile`, não criamos conta de serviço, instalação PostgreSQL ou cluster, nem alteramos firewall, política TLS, configuração `pg_hba.conf`, bases ou esquemas. A autenticação das versões/assinaturas vem do gate de pacotes offline; o comportamento de provisionamento/ACL da conta Windows precisa de ensaio em ambiente descartável antes de qualquer automação efetiva. Referência dos parâmetros: [documentação oficial EDB](https://www.enterprisedb.com/docs/supported-open-source/postgresql/installing/command_line_parameters/). O runner Windows testa geração e recusas com senhas **100% sintéticas**, sem executar instaladores de terceiros.

### Criar a configuração local do Garnet sem iniciar um serviço

A ferramenta `tools/provision-esterium-garnet-config.ps1` é a primeira etapa com gravação **real e explicitamente autorizada** (`-Apply`) do preparo do cache para uma **instalação nova**. Antes disso, exige três arquivos externos previamente provisionados e com permissões seguras: um arquivo ACL Garnet com **usuário default desabilitado, nenhum `nopass` e uma conta nomeada com senha**, um certificado TLS válido `.pfx` com chave privada e um arquivo contendo **somente a senha do PFX**, todos legíveis exclusivamente por `NETWORK SERVICE`, `SYSTEM` e Administradores. A conta `NETWORK SERVICE` não pode alterar esses arquivos. O comando exige privilégios de administrador e recusa sobrescrever uma configuração existente.

O padrão é apenas um plano **sem alterações**. Com `-Apply`, a ferramenta cria `ProgramData\\Esterium\\GarnetConfig\\garnet.conf` protegido com ACLs Windows, contendo `Address=127.0.0.1`, `AuthenticationMode=ACL`, `EnableTLS=true`, `EnableLua=true` e `LuaTransactionMode=true`, com limites de memória e execução de scripts Lua. A senha do PFX vai **somente para o JSON protegido**, porque o Garnet requer `CertPassword`; ela **não aparece na linha de comando, logs, console ou no ZIP**.

Exemplo para o técnico, após criar os arquivos de modo seguro e verificar os certificados:

```powershell
.\\tools\\provision-esterium-garnet-config.ps1 `
  -AclFile 'C:\\ProgramData\\Esterium\\GarnetSecrets\\users.acl' `
  -CertificatePfxFile 'C:\\ProgramData\\Esterium\\GarnetSecrets\\tls.pfx' `
  -CertificatePasswordFile 'C:\\ProgramData\\Esterium\\GarnetSecrets\\pfx-password.secret'
```

**Não é instalação completa nem um certificado de compatibilidade:** este utilitário não baixa Garnet, não cria contas do Windows, não registra ou inicia seu serviço, não altera firewall ou banco PostgreSQL, não emite certificados e não testa uma conexão TLS. Uma futura instalação deve usar somente um binário Garnet de procedência aprovada e validar o `garnet.conf` em execução sob `NETWORK SERVICE`, seguido da verificação das operações Redis/Lua do Esterium. Para execução, o Garnet documenta `GarnetServer.exe --config-import-path <caminho>` em [configuração oficial](https://microsoft.github.io/garnet/docs/getting-started/configuration). A senha no JSON reforça a obrigação de controlar ACL e ciclo de vida desse arquivo.

O CI Windows usa um certificado PFX temporário, uma ACL e senha **100% fictícias** em um runner hospedado descartável. Verifica que o plano não grava, que ACLs inseguras são rejeitadas e que a geração explícita não altera serviços nem dados comerciais.

### Pacotes offline PostgreSQL e Garnet: verificação de procedência antes de instalar

O utilitário `tools/check-esterium-offline-dependency-packages.ps1` confere **arquivos externos já baixados por um técnico** e não os executa. Requer o instalador Windows PostgreSQL em `.exe`, sua assinatura Authenticode válida sob um certificado de editor **previamente aprovado**, e os hashes SHA-256 PostgreSQL e Garnet obtidos **por um canal confiável independente** (nunca a partir do mesmo ZIP que se pretende validar). Para Garnet, aceita `.exe` assinado com identidade aprovada, ou um ZIP auditado por hash e por nomes de arquivo, **sem extrair**, destacando que ZIP com hash correto ainda precisa de aprovação independente da origem do fornecedor.

Os parâmetros obrigatórios são `-PostgresInstaller`, `-ExpectedPostgresSHA256`, `-ExpectedPostgresPublisherThumbprint`, `-GarnetPackage` e `-ExpectedGarnetSHA256`; quando Garnet for EXE, exige também `-ExpectedGarnetPublisherThumbprint`. Os digests e certificados confiáveis devem ser fornecidos pelo responsável de segurança, não copiados automaticamente do pacote de instalação. Nenhum arquivo de terceiros é distribuído no bundle atual. O teste automatizado cria apenas pacotes fictícios em runner descartável, sem rede ou dados da loja.

Para ZIPs, o gate também rejeita nomes duplicados/collisions de maiúsculas e minúsculas, entradas do tipo link simbólico, caminhos que escapariam da extração e arquivos compactados com razão de compressão ou tamanho total excessivos. Essas verificações examinam apenas metadados ZIP, **não extraem** arquivos e não garantem que um extrator posterior seja seguro. A liberação de um pacote precisa de procedência aprovada independentemente e inspeção de seu conteúdo.

**Limite importante:** essa verificação de origem não instala PostgreSQL/Garnet, não aprova versões, não cria serviços, não configura TLS/credenciais, não valida compatibilidade Redis Lua e não serve como comprovante de homologação. É uma barreira adicional antes de construir um provisionador local assinado.

### Compatibilidade do Garnet (opção Windows sem servidor de nuvem)

O **Garnet** da Microsoft é uma alternativa local de código aberto compatível com parte do protocolo Redis, mas **não é uma substituição perfeitamente idêntica**. A documentação oficial indica suporte a scripts Lua com `--lua`, que não devem ser considerados habilitados sem configuração. O backend Esterium precisa de operações atômicas Redis para sessões e limitação de tentativas: a compatibilidade efetiva **depende da aprovação de `--check-dependencies` no serviço real**, incluindo scripts e semântica de consumo único; o inventário de portas não comprova isso.

**Segurança essencial:** a configuração padrão documentada do Garnet pode escutar em todas as interfaces IPv4/IPv6, e isso **não serve para o servidor da loja**. O operador deve vinculá-lo somente a `127.0.0.1` / `::1`, habilitar autenticação forte e TLS com certificado PFX protegido por ACL, sem senha em linha de comando ou ZIP. O Windows/firewall precisa impedir acesso direto pela LAN, enquanto os caixas acessam **apenas o HTTPS do Esterium**. O teste local bloqueia escutas não-loopback, mas não comprova configuração TLS nem identidades de processo.

O suporte declarado no site oficial não substitui ensaio de reinício, persistência, recuperação e revogação de sessão no ambiente real. Para referência: [compatibilidade e Lua](https://microsoft.github.io/garnet/docs/welcome/compatibility), [orientação de execução](https://microsoft.github.io/garnet/docs/getting-started) e [segurança/TLS](https://microsoft.github.io/garnet/docs/security). **Nenhum instalador ou configuração automática do Garnet está incluído neste PR.**

### Conferir se as portas locais pertencem aos serviços declarados

O inventário `tools/check-esterium-local-dependencies.ps1` aceita `-RequireServiceOwnership` para uma conferência mais rigorosa: os processos donos das portas locais PostgreSQL e Redis/Garnet precisam corresponder ao identificador do processo declarado por cada serviço Windows. Isso ajuda a detectar uma aplicação indevida escutando `127.0.0.1` nas portas esperadas. O instalador `tools/install-esterium-local.ps1 -Apply` agora exige essa conferência, enquanto a consulta de planejamento permanece disponível sem ela para diagnóstico inicial.

**Limite:** serviços gerenciados por wrappers que iniciem processos filhos podem não passar nesse teste. Nesse caso, o instalador falha fechado; não se deve desabilitar a exigência sem definir uma instalação segura e verificável. Mesmo quando aprovado, esse teste não prova autenticidade TLS, versão PostgreSQL, permissões SQL nem compatibilidade Lua/Redis. A validação de dependências do backend continua obrigatória.

### Conferir PostgreSQL e Redis já instalados, sem modificar serviços

O utilitário `tools/check-esterium-local-dependencies.ps1` **não instala** PostgreSQL nem o serviço Redis compatível (por exemplo, Garnet). Depois que um técnico provisionar esses componentes Windows com privilégios mínimos e autenticação/TLS próprios, ele confere apenas: nomes específicos dos dois serviços em execução, caminhos das ferramentas PostgreSQL existentes (incluindo `pg_restore`), portas distintas e escutas TCP **exclusivamente em loopback**. É um levantamento somente leitura, sem iniciar processos, abrir conexões ou examinar dados comerciais.

Exemplo para um ambiente de testes (substitua os nomes *reais* encontrados nos Serviços do Windows):

```powershell
.\tools\check-esterium-local-dependencies.ps1 `
  -PostgresServiceName 'postgresql-x64-16' `
  -RedisServiceName 'EsteriumGarnet' `
  -PostgresBin 'C:\Program Files\PostgreSQL\16\bin' `
  -PostgresPort 5432 -RedisPort 6379
```

Um resultado positivo **não comprova que o processo que escuta a porta pertence ao serviço informado**, nem versões, suporte do Garnet, conexões TLS, senha, migrações, proteção da pasta de dados ou compatibilidade real dos recursos atômicos do Redis. Essas verificações exigem o diagnóstico do próprio backend, `esterium-server.exe --check-config` e `--check-dependencies`, com URLs e certificados confiáveis de verdade. O segundo faz operações sintéticas de curta duração no Redis, mas não inicia vendas ou migrações. Redis e PostgreSQL **não devem ficar expostos à LAN** só porque o PDV roda em mais de um caixa.

### Antes de executar (ainda em ambiente descartável)

- Separar `DATABASE_URL_FILE`, `REDIS_URL_FILE`, `JWT_SECRET_FILE` e `METRICS_BEARER_TOKEN_FILE` fora da pasta do programa. O conteúdo deve ser restrito por ACL à conta de serviço autorizada e aos administradores; os arquivos **não** são produzidos pelo script.
- Garantir conexões `sslmode=verify-full` e `rediss://` com certificado e credencial fortes. Testar a inicialização fail-closed do PR #63 antes de utilizar.
- Verificar/assinar a origem do pacote Windows. Um ZIP de CI não é automaticamente uma distribuição assinada.
- Garantir `CORS_ALLOWED_ORIGINS` adequado ao ambiente prod e os demais controles exigidos pelo carregador de configurações; esse registro inicial **não configura** todos os requisitos de inicialização.
- Não iniciar com dados reais nem liberar SEFAZ; `FISCAL_PROVIDER=disabled` e flags de transmissão fiscal desativadas.

Extraia o artefato técnico em uma pasta temporária confiável e abra o PowerShell **nessa pasta**, onde ficam `esterium-server.exe`, `web/`, `tools/` e este `README_LOCAL.md`. Se estiver trabalhando no clone do repositório, o comando equivalente fica em `scripts/register-esterium-windows-service.ps1` e requer `-BundleDir` apontando para o pacote compilado.

A simulação **sem alterações no Windows** deve sempre ser executada antes da instalação real:

```powershell
.\tools\register-esterium-windows-service.ps1 `
  -BundleDir (Get-Location).Path `
  -DatabaseURLFile 'C:\ProgramData\Esterium\Secrets\database-url.secret' `
  -RedisURLFile 'C:\ProgramData\Esterium\Secrets\redis-url.secret' `
  -JWTSecretFile 'C:\ProgramData\Esterium\Secrets\jwt.secret' `
  -MetricsBearerTokenFile 'C:\ProgramData\Esterium\Secrets\metrics.secret' `
  -AllowedOrigin 'https://esterium.loja.example' `
  -PlanOnly
```

A origem HTTPS é um exemplo de formato: forneça o nome **verdadeiro e validado** que será usado na loja e configure certificado/proxy apropriado antes de servir terminais pela rede. O registro em si continua limitado ao loopback e não substitui o PR #66 de HTTPS LAN.

**Observação:** os caminhos acima são exemplos; o script exige que todos os arquivos já existam e recusa caminhos ausentes. A etapa real requer administrador e explicitamente remover `-PlanOnly`. O serviço criado não será iniciado automaticamente. Não utilizar esse fluxo enquanto credenciais, ACLs, migrações e restauração não estiverem validados.

O job Windows `local-windows-installer-plan` testa com **arquivos sintéticos** que a simulação não cria instalação, rejeita credencial ausente e recusa substituir pasta existente. Ele não instala serviço real em ambiente de produção, não executa restauração nem disponibiliza o ERP fiscalmente.

## Pré-verificação do executável sem acessar banco

O executável agora admite exclusivamente `--check-config`, que lê e valida as variáveis/arquivos de configuração, a correspondência do certificado HTTPS e o frontend compilado **sem abrir portas, conectar ao PostgreSQL/Redis ou transmitir à SEFAZ**. Também faz essa conferência básica antes das conexões no início normal. O CI Windows executa o preflight com arquivos e URLs inteiramente sintéticos e recusa argumentos inválidos.

A segunda verificação, `esterium-server.exe --check-dependencies`, consulta somente a versão e integridade das migrações, confere 11 tabelas exigidas, testa TLS e acesso PostgreSQL e exige **consumo único e limitação de tentativas atômicos no Redis**. Ela não inicia o servidor HTTP nem executa migrações, vendas ou transmissão SEFAZ. O teste de segurança Redis cria identificadores sintéticos únicos de curta duração (o token de sessão é revogado ao fim). Configure o ambiente e a identidade do serviço corretamente antes de executá-la; a aprovação desse comando demonstra conectividade e operações básicas, não disponibilidade dos caixas nem restauração.

**Importante:** executar manualmente o `.exe` fora do Gerenciador de Serviços não carrega automaticamente as entradas `Environment` do serviço no registro do Windows. É necessário executar o preflight na identidade do serviço ou em ambiente controlado equivalente, sem imprimir ou copiar valores de segredos. A aprovação desse teste não demonstra que o banco ou os certificados são alcançáveis.

## Migrações incluídas no pacote

O ZIP agora inclui `migrations/` com os arquivos SQL `.up.sql` e `.down.sql` versionados que pertencem à release. O manifesto SHA-256 impede que faltem pares ou que esses arquivos sejam modificados durante a cópia. O instalador conserva a pasta junto ao servidor e registra o caminho em `ESTERIUM_MIGRATIONS_DIR`.

No comando `--check-dependencies`, quando essa pasta está configurada, o Esterium verifica que as versões são contínuas e que a versão atual de `schema_migrations` é **exatamente igual** à versão máxima do pacote. Um banco atrasado, adiantado ou marcado como `dirty` é recusado. **O comando não executa migrações automaticamente.** O ZIP passa a incluir `tools/esterium-migrate.exe`, CLI do projeto golang-migrate compilado para Windows x64 a partir da versão fixa `v4.17.1` (mesma família da ferramenta de migração usada no CI), protegido pelo manifesto SHA-256 tanto antes quanto depois do registro do serviço. O CLI é uma ferramenta técnica, **não é executado pelo instalador** e não deve ser usado manualmente com o banco da loja sem um procedimento revisado de migração, validação de backups externos e parada dos caixas. Ele admite comandos destrutivos e ainda não há uma automação segura de atualização para produção. A distribuição não é uma assinatura de procedência; versão/assinatura do pacote completo permanecem pendentes.

## Integridade e atualização do backup externo

O pacote também instala `tools/check-esterium-backup-integrity.ps1`. Ele verifica em **modo somente leitura** se o backup local e a cópia externa SMB são da mesma geração, possuem arquivos SHA-256 válidos e foram concluídos dentro do prazo de 30 horas por padrão. A execução falha diante de backup ausente, remoto desconectado, corrompido ou antigo. O status do backup salva apenas nome e hash do arquivo, sem senhas, dados fiscais ou informações de clientes. O CI Windows testa adulteração, ausência de checksum e expiração em arquivos sintéticos.

O pacote também inclui um registrador protegido de backup diário Windows, **desabilitado por padrão**, e um executor que exige backup externo SMB e PostgreSQL TLS com segredos em arquivos de ACL restrita. Um técnico deve preparar o JSON de configuração fora do programa, testar a identidade NetworkService, a escrita na pasta remota e restauração em outra máquina **antes de habilitar o agendamento**. Consulte `docs/esterium-local-backup-externo.md` no repositório. O verificador sozinho não agenda nem restaura o banco.

## Desregistro seguro do serviço (sem apagar dados)

O pacote técnico inclui `tools/unregister-esterium-windows-service.ps1`. O modo padrão verifica **sem alterações** se existe o serviço `EsteriumERP`, se está parado, se aponta para o `esterium-server.exe` da pasta indicada, se utiliza LocalService e se a tarefa de backup está desabilitada ou ausente. Recusa atingir um serviço desconhecido, em execução, com caminho diferente ou com backup ativo.

Após reconciliar as vendas, garantir cópia e recuperação dos dados e parar o serviço manualmente, o responsável técnico pode usar explicitamente `-Apply` com privilégios de administrador **somente para remover o registro SCM**. A pasta de programa, o banco PostgreSQL, Redis, os arquivos fiscais, os segredos, os backups e a tarefa de backup **são preservados**. A retirada completa do programa e a limpeza de dados exigem outro procedimento futuro e autorização específica.

Exemplo do modo de leitura:

```powershell
.\tools\unregister-esterium-windows-service.ps1 -InstallDir 'C:\Program Files\EsteriumERP'
```

O CI Windows hospeda um serviço e uma tarefa sintéticos, exercita o plano, impede desregistro por caminho divergente, remove apenas o serviço e verifica que arquivos e backup foram preservados. Isso **não** equivale a uma desinstalação completa de loja real.

## Atualização controlada somente do programa (sem banco de dados)

O pacote contém `tools/apply-esterium-app-update.ps1`, com plano de leitura por padrão e `-Apply` administrativo explícito. Ele serve **exclusivamente** para atualizações do executável Go, telas React e ferramentas quando **todas as migrações SQL continuam exatamente iguais**. Se uma versão SQL nova for necessária, o comando recusa a operação: uma atualização completa com migração e teste de restauração continua pendente.

Antes de qualquer troca, exige os dois pacotes íntegros, serviço parado, agendamento de backup desabilitado e backup local/externo fresco e verificado. Rejeita pacote sem assinatura Authenticode fora dos testes descartáveis do GitHub. Ao aplicar, copia o candidato para uma pasta temporária sob Program Files, verifica hashes, reconfere as condições, move a pasta antiga para um **diretório de rollback preservado** e coloca a nova no mesmo local. Uma falha de troca tenta restaurar a pasta anterior; se a recuperação automática falhar, retém os arquivos e avisa para intervenção técnica. O serviço não é iniciado.

O pacote atual continua sem assinatura e o fluxo **não pode ser utilizado em uma loja real**. O smoke do CI simula pacotes e backup, recusando migrações adicionais e testando a troca/retenção do pacote anterior sem banco real. Nenhum passo deste procedimento acessa clientes, produtos, vendas, equipamentos periféricos ou SEFAZ.

## Verificação de atualização sem escrever no banco ou no programa

O pacote inclui `tools/plan-esterium-windows-upgrade.ps1`. Ele serve **somente para planejar** uma futura atualização, exigindo que a instalação atual e o novo pacote tenham arquivos intactos e versões SQL antigas preservadas, sem modificação retroativa. Exige também serviço `EsteriumERP` já **parado**, tarefa diária de backup **desabilitada** e cópia local + externa comprovadamente recente e íntegra. Não acessa PostgreSQL/Redis e **não executa migrações, cópia de binários, instalação, rollback de dados nem reinício**.

Antes da distribuição comercial, a opção padrão recusa o novo executável sem assinatura Authenticode válida. Como o pacote técnico ainda não é assinado, `-TechnicalUnsignedPlan` permite **apenas um plano explícito de laboratório**, com aviso de que SHA-256 não garante procedência. Mesmo uma assinatura válida do executável ainda não comprova a identidade de todos os scripts/artefatos da release; a cadeia de assinatura completa continua pendente.

O procedimento de laboratório exige caminhos reais e **uma cópia SMB em outro computador**:

```powershell
.\tools\plan-esterium-windows-upgrade.ps1 `
  -InstalledDir 'C:\Program Files\EsteriumERP' `
  -CandidateBundleDir 'C:\Esterium\PacoteNovo' `
  -BackupDir 'D:\Esterium\Backups' `
  -ExternalBackupDir '\\computador-reserva\esterium' `
  -TechnicalUnsignedPlan
```

Os scripts de validação exigem que **ambas as instalações contenham o conjunto de utilitários compatível com o verificador atual**. Instalações mais antigas que não tenham essas ferramentas devem seguir um procedimento manual de migração, nunca ser forçadas a passar por esse gate. Mesmo com resultado positivo, é indispensável ensaiar recuperação em outro PC, criar um atualizador transacional, controlar a origem do pacote e obter aceite presencial antes de atualizar dados de loja.

## Pendente para instalador completo

Provisionar PostgreSQL e backend Redis/Garnet com TLS e ACL, segredo por DPAPI/Credential Manager, `APP_PUBLIC_URL`/CORS reais, certificado HTTPS confiável para LAN, isolamento no firewall, migrações em banco novo, atualização sem perda de dados, validação/ativação presencial da tarefa e restauração real de backups, inicialização automática e desinstalação preservando dados.
