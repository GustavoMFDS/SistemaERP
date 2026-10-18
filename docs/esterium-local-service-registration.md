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

## Segurança de instalação

O programa **recusa sobrescrever** a pasta de instalação ou o serviço `EsteriumERP` já existentes. Copia o executável compilado, `web/`, `tools/`, `README_LOCAL.md` e `bundle-manifest.json`; os arquivos são verificados por SHA-256 **novamente na pasta temporária antes do registro SCM**. Nenhum valor de credencial, banco ou backup é copiado. As ferramentas de manutenção ficam disponíveis na pasta instalada, mesmo depois de descartar o ZIP extraído; sua execução e o agendamento de backup continuam procedimentos separados.

O pacote deve incluir `bundle-manifest.json`: todos os arquivos do executável e `web/` são comparados com seus SHA-256 e arquivos extras/links simbólicos são rejeitados **antes da instalação**, inclusive durante `-PlanOnly`. O manifesto é gerado pelo script de build e conferido no CI; ele verifica integridade relativa ao manifesto, **não comprova origem/autenticidade** (assinatura de código ainda pendente).

Se `sc.exe create` der certo, mas a gravação do ambiente no Registro ou a atribuição da identidade de serviço falhar, o registrador tenta **remover somente o serviço criado naquela invocação**. Os arquivos copiados permanecem para inspeção manual; não são apagados dados existentes, credenciais ou outros serviços. O CI Windows injeta uma falha descartável após `sc create`, verifica a reversão e só depois testa o registro normal. Uma falha na própria exclusão gera aviso explícito para intervenção técnica, nunca sucesso.

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

## Pendente para instalador completo

Provisionar PostgreSQL e backend Redis/Garnet com TLS e ACL, segredo por DPAPI/Credential Manager, `APP_PUBLIC_URL`/CORS reais, certificado HTTPS confiável para LAN, isolamento no firewall, migrações em banco novo, atualização sem perda de dados, validação/ativação presencial da tarefa e restauração real de backups, inicialização automática e desinstalação preservando dados.
