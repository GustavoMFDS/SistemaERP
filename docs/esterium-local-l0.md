# Esterium Local — prova de distribuição L0 (Windows x64)

Este marco **não é o instalador** e não libera o uso em produção. Seu objetivo é comprovar que a interface compilada pode ser servida diretamente por um executável Go e consumir a API **na mesma origem**, sem servidor Vite nem Node em execução na loja.

## Compilar (na máquina de desenvolvimento)

No PowerShell, na raiz do projeto:

```powershell
.\scripts\build-esterium-local-windows.ps1
```

O script exige Go e Node/npm **somente na máquina de compilação** e gera uma pasta nova em `dist/esterium-local-win-x64/`, contendo `esterium-server.exe` e `web/index.html` com os arquivos estáticos. Ele não sobrescreve uma pasta preexistente. Não incorpora `.env`, chaves, senhas, certificados, banco de dados nem seed demonstrativo.

## Prova técnica local (somente desenvolvimento ou teste)

O servidor deve ter PostgreSQL, Redis, migrações e variáveis seguras configuradas **externamente** ao bundle. A partir da pasta compilada:

```powershell
$env:ESTERIUM_WEB_DIR = (Join-Path (Get-Location) 'web')
$env:HTTP_ADDR = '127.0.0.1:8080'
# Configurar APP_ENV, DATABASE_URL, REDIS_*, JWT_SECRET e outros
# valores de teste por meio do gerenciador de segredos apropriado.
.\esterium-server.exe
```

Abrir `http://127.0.0.1:8080` deve mostrar a UI e utilizar `/api/v1` na mesma origem. Se a pasta `web` ou `index.html` estiver ausente, o executável recusa a inicialização em vez de mostrar uma aplicação incompleta.

Com `ESTERIUM_WEB_DIR` ausente, o servidor mantém o comportamento de API-only anterior. A execução local de desenvolvimento com Vite em `:5173` continua usando a API `:8080` por padrão.

## Proteções do servidor estático

- Rotas `/api`, `/health`, `/metrics` e `/debug` permanecem com a API; nunca retornam `index.html` como resposta de fallback.
- Um arquivo JS/CSS ausente retorna 404; apenas navegação HTML de páginas sem extensão utiliza o fallback da SPA.
- Arquivos com links simbólicos para fora do bundle não são servidos.
- `index.html` não é cacheado; recursos estáticos versionados em `assets/` podem ser cacheados.
- O recurso só aceita GET/HEAD; não transforma POST/PUT em navegação HTML.

## Ainda necessário para instalação de loja

L1: serviços locais com PostgreSQL e mecanismo equivalente ao Redis **sem enfraquecer** rotação/revogação e limite de requisições. L2: instalador/serviço Windows e segredos automáticos. L3: HTTPS confiável e validação de dois caixas via LAN. L4: backup externo com restauração ensaiada e desligamento abrupto. L5/L6: testes reais de venda, impressoras, estoque, usuários, atualização, desinstalação com preservação de dados e PC limpo. L7: Linux depois do Windows.

**Fiscal:** manter a transmissão SEFAZ desabilitada até credenciamento, certificado e homologação reais. Aprovação de L0 e CI não significa liberação fiscal ou operacional.
