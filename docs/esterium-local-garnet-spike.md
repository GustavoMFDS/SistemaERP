# Esterium Local — investigação Garnet como serviço de sessões no Windows

## Por que investigar

Em modo produtivo, o ERP depende de um backend Redis seguro. O armazenamento de refresh tokens usa `SET`, `EVAL` e `DEL` para **consumo atômico**; o limitador de requisições usa script com `INCR`, `PTTL` e `PEXPIRE`, com fechamento por segurança quando o backend está indisponível.

O [Garnet da Microsoft](https://github.com/microsoft/garnet) é um cache-store de código aberto (licença MIT) compatível com protocolo RESP, com suporte a Windows. Documentação: [compatibilidade](https://microsoft.github.io/garnet/docs/welcome/compatibility), [Lua/transactions](https://microsoft.github.io/garnet/docs/getting-started/configuration) e [persistência](https://microsoft.github.io/garnet/docs/welcome/features).

## Prova automatizada no CI, não uma liberação

O job Windows `local-garnet-windows-compat` baixa **somente o release oficial v2.2.3**, valida o SHA-256 de `win-x64-based-readytorun.zip` e inicia Garnet em **127.0.0.1**, porta de teste 16379. A primeira etapa isola **somente o contrato RESP/Lua transacional** (`--lua --lua-transaction-mode`). Persistência AOF e recuperação após falhas serão testes **separados e obrigatórios** antes de qualquer implantação.

Ele executa `TestLocalGarnetRedisSecurityContract` usando o **código real do backend Esterium** para verificar:

- refresh token consumido exatamente uma vez sob concorrência de 16 requisições;
- token inválido ou revogado não pode ser reutilizado;
- limitador atômico permite exatamente 5 em 16 tentativas simultâneas e falha fechado quando backend não está disponível;
- expiração do token após TTL real.

O processo de teste é terminado ao final da execução. Nenhuma senha operacional é baixada ou embutida. O CI **não altera** a configuração `DISABLE_REDIS` e não muda a autenticação de produção.

**Resultado adverso observado:** no primeiro run Windows com `--aof --aof-commit-wait --recover --fail-on-recovery-error`, o Garnet registrou erros de leitura do log e as operações reais de autenticação/limitação expiraram. A investigação isolou AOF do teste de comandos. Isso NÃO é compatibilidade produtiva comprovada.

## O que ainda falta antes de adotar Garnet

1. Repetir a compatibilidade no mesmo binário Windows que será distribuído, com TLS autenticação/ACL e credenciais geradas pelo instalador.
2. Demonstrar persistência de refresh tokens, consumo único e rate limit após reinício **limpo e desligamento abrupto**, e avaliar AOF/checkpoints antes de liberar.
3. Avaliar comportamento de concorrência sob carga real e queda de disco; o `--lua-transaction-mode` precisa estar ativo. Se qualquer verificação falhar, a aplicação deve **recusar** início ou login, nunca cair para o limitador apenas em memória.
4. Verificar assinatura/licença, dependências .NET, política de atualização, execução como serviço Windows restrito, certificado e isolamento em loopback.
5. Integrar com instalação PostgreSQL, migrações, backup externo, LAN HTTPS e homologação com dois caixas. O bundle L0 atual **não contém Garnet**.

**Resultado do CI não transforma Garnet automaticamente em Redis 100% compatível.** Até aceite dos pontos acima, o backend mantém a dependência atual e o ERP local não está pronto para distribuição de produção. Emissão fiscal SEFAZ continua pendente de homologação.
