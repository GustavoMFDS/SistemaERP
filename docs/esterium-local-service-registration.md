# Registro do serviço Windows — Esterium Local (L2 parcial)

O script `scripts/register-esterium-windows-service.ps1` registra **apenas** o executável Go e os arquivos compilados da interface em `C:\Program Files\EsteriumERP`. Ele **não é um instalador completo** de loja.

## Segurança de instalação

O programa **recusa sobrescrever** a pasta de instalação ou o serviço `EsteriumERP` já existentes. Copia apenas o executável compilado e a pasta `web`; nunca inclui arquivo de credenciais, banco ou backup.

A conta padrão do serviço é **LocalService**, sem privilégios administrativos de rotina. O serviço é registrado com inicialização **manual** e só deve ser iniciado depois que os bancos, os certificados TLS de PostgreSQL/Redis e as permissões dos arquivos estejam configurados e as migrações tenham sido aplicadas. O código Go aceita Stop/Shutdown do Gerenciador de Serviços.

### Antes de executar (ainda em ambiente descartável)

- Separar `DATABASE_URL_FILE`, `REDIS_URL_FILE`, `JWT_SECRET_FILE` e `METRICS_BEARER_TOKEN_FILE` fora da pasta do programa. O conteúdo deve ser restrito por ACL à conta de serviço autorizada e aos administradores; os arquivos **não** são produzidos pelo script.
- Garantir conexões `sslmode=verify-full` e `rediss://` com certificado e credencial fortes. Testar a inicialização fail-closed do PR #63 antes de utilizar.
- Verificar/assinar a origem do pacote Windows. Um ZIP de CI não é automaticamente uma distribuição assinada.
- Garantir `CORS_ALLOWED_ORIGINS` adequado ao ambiente prod e os demais controles exigidos pelo carregador de configurações; esse registro inicial **não configura** todos os requisitos de inicialização.
- Não iniciar com dados reais nem liberar SEFAZ; `FISCAL_PROVIDER=disabled` e flags de transmissão fiscal desativadas.

A simulação **sem alterações no Windows** deve sempre ser executada antes da instalação real:

```powershell
.\scripts\register-esterium-windows-service.ps1 `
  -BundleDir 'D:\Artefatos\esterium-local-l0-win-x64' `
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
