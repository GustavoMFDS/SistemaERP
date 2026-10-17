# Registro do serviço Windows — Esterium Local (L2 parcial)

O script `scripts/register-esterium-windows-service.ps1` registra **apenas** o executável Go e os arquivos compilados da interface em `C:\Program Files\EsteriumERP`. Ele **não é um instalador completo** de loja.

**Novo modo LAN:** opcionalmente aceite `-ListenAddress '0.0.0.0:8443' -TLSCertFile 'C:\ProgramData\Esterium\TLS\server-cert.pem' -TLSKeyFile 'C:\ProgramData\Esterium\TLS\server-key.pem'`. Um endereço não-local **sempre exige o par TLS**, guardado fora do ZIP e da pasta do programa, com permissão de leitura explícita para `LocalService`. O Go verifica par/certificado, validade e se o nome do certificado (SAN) corresponde ao `APP_PUBLIC_URL` informado; exige TLS 1.3 e recusa iniciar se houver divergência. Os navegadores dos caixas ainda precisam confiar na cadeia do certificado, e a regra de firewall LAN deve ser configurada independentemente.

O padrão permanece `127.0.0.1:8080` sem TLS, acessível apenas no próprio PC ou por proxy HTTPS externo. O registrador não configura proxy, DNS, firewall ou confiança dos clientes; tampouco substitui a revisão do `APP_PUBLIC_URL` utilizado no acesso real.

## Segurança de instalação

O programa **recusa sobrescrever** a pasta de instalação ou o serviço `EsteriumERP` já existentes. Copia apenas o executável compilado e a pasta `web`; nunca inclui arquivo de credenciais, banco ou backup.

O pacote deve incluir `bundle-manifest.json`: todos os arquivos do executável e `web/` são comparados com seus SHA-256 e arquivos extras/links simbólicos são rejeitados **antes da instalação**, inclusive durante `-PlanOnly`. O manifesto é gerado pelo script de build e conferido no CI; ele verifica integridade relativa ao manifesto, **não comprova origem/autenticidade** (assinatura de código ainda pendente).

A conta padrão do serviço é **LocalService**, sem privilégios administrativos de rotina. O serviço é registrado com inicialização **manual** e só deve ser iniciado depois que os bancos, os certificados TLS de PostgreSQL/Redis e as permissões dos arquivos estejam configurados e as migrações tenham sido aplicadas. O código Go aceita Stop/Shutdown do Gerenciador de Serviços.

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

## Pendente para instalador completo

Provisionar PostgreSQL e backend Redis/Garnet com TLS e ACL, segredo por DPAPI/Credential Manager, `APP_PUBLIC_URL`/CORS reais, certificado HTTPS confiável para LAN, isolamento no firewall, migrações em banco novo, atualização sem perda de dados, agendamento e restauração real de backups, inicialização automática e desinstalação preservando dados.
