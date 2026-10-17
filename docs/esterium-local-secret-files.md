# Esterium Local — URLs de conexão por arquivo protegido (L1)

O backend aceita agora `DATABASE_URL_FILE` e `REDIS_URL_FILE`, além de `JWT_SECRET_FILE` e `REDIS_PASSWORD_FILE` que já existiam. Esse suporte permite apontar o serviço Windows para arquivos de configuração **com ACL restrita**, sem inserir a senha de PostgreSQL ou do armazenamento de sessões em argumentos de processo.

## Regras

- `DATABASE_URL` tem precedência sobre `DATABASE_URL_FILE`; `REDIS_URL` tem precedência sobre `REDIS_URL_FILE`. Se a variável com o valor estiver vazia e o arquivo estiver indicado, o arquivo é lido integralmente e espaços/linhas finais são removidos.
- Se um arquivo indicado estiver ausente, for diretório ou não puder ser lido, a inicialização falha; não há troca silenciosa para senha padrão.
- No Windows, guardar os arquivos em diretório **fora do pacote do aplicativo** (por exemplo, área restrita em ProgramData), com permissões para a identidade dedicada do serviço e administradores autorizados. Evitar herança de permissões para `Users`/operadores e não colocar esses arquivos em pastas compartilhadas ou no repositório.
- Os arquivos contêm segredos em texto e **ACL não é criptografia**. O instalador deverá acrescentar DPAPI/Credential Manager ou proteção equivalente, controle de atualização, restauração, auditoria e restrição de leitura.
- Em modo produtivo, permanece obrigatório `sslmode=verify-full` para PostgreSQL, URL `rediss://` com senha válida para Redis/Garnet, expiração curta de access token e verificação atômica de sessão/rate limit do PR #63.
- Nenhum valor ou conteúdo de arquivo deve aparecer em logs, captura de tela, linha de comando, atalhos, ZIP de distribuição ou backup externo sem criptografia.

## Status de entrega

Esta alteração só cobre leitura e validação de arquivos de segredos. **Não cria segredos, permissões ou os serviços automaticamente**, e não conclui o instalador. A emissão fiscal SEFAZ continua bloqueada até homologação.
