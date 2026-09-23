# CI self-hosted do SistemaERP

## Estado observado

Os runners Windows usados pelo repositório `Mizuki-TheSeal` não são elegíveis para jobs do `SistemaERP`.

Evidência operacional em 2026-09-23:

- `GUSTAVO` (runner id 23) executou com sucesso um job do Mizuki usando os labels `self-hosted, Windows, X64, unreal-5.8`.
- Enquanto isso, os jobs self-hosted do SistemaERP permaneceram sem `runner_id`/`runner_name`, mesmo quando usavam os mesmos labels.
- O fallback para `ubuntu-latest` continua falhando antes de qualquer step, com `steps: 0`.

Portanto, o cutover confiável requer runners registrados para este repositório.

## Registro recomendado

No GitHub, abra:

`SistemaERP > Settings > Actions > Runners > New self-hosted runner`

Selecione Windows / x64 e siga os comandos fornecidos pelo próprio GitHub. O token de registro é temporário; não o salve no repositório.

Na máquina Windows, use um terminal com privilégios de administrador se o runner for instalado como serviço.

Use diretórios independentes dos runners do Mizuki, por exemplo:

- `C:\actions-runner-sistemaerp-01`
- `C:\actions-runner-sistemaerp-02`

Registre os dois como serviços separados. Não reutilize nem copie um diretório já configurado para outro repositório.

## Labels

Para o ERP, use somente os labels padrão:

```yaml
runs-on: [self-hosted, Windows, X64]
```

O label `unreal-5.8` pertence ao pipeline do Mizuki e não deve ser requisito do SistemaERP.

## Pré-requisitos do host

Antes de executar a suíte completa, os serviços precisam conseguir acessar:

- Git e GitHub por HTTPS;
- Docker com Linux containers e `docker compose`;
- portas locais usadas pelo CI (`5173`, `8080`, `18080`, `18081`, `5433`, `55432`, `6379`, `56379`, `8443`);
- espaço para imagens Docker e browsers do Playwright.

Go e Node podem ser provisionados pelos actions `setup-go` e `setup-node`, mas Docker precisa estar disponível para a conta do serviço que executa o runner.

## Critério de aceite do cutover

Só considerar o runner funcional quando um run do mesmo HEAD mostrar:

1. `runner_id` e `runner_name` preenchidos;
2. checkout executado;
3. os seis gates realmente rodando: `backend`, `frontend`, `integration`, `e2e`, `e2e-prodlike`, `security`;
4. os seis gates verdes no mesmo commit.

Até isso acontecer, não usar `steps: 0` como evidência de validação de código.

## Referência

GitHub Docs: https://docs.github.com/en/actions/how-tos/manage-runners/self-hosted-runners/add-runners
