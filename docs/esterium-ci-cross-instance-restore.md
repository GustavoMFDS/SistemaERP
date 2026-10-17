# Esterium Local — teste de restauração entre instâncias independentes

O job de integração cria um backup custom-format do banco PostgreSQL **descartável do CI** e o restaura em um **segundo servidor PostgreSQL 16**, não apenas em outro banco da mesma instância.

Essa segunda instância usa contêiner próprio, sem volume compartilhado com a origem, sem rede (`--network none`) e sem portas publicadas. O teste exige runner GitHub hospedado, nome exclusivo por execução, fonte com nome exato `sistemaemgo`, migrações não pendentes e inexistência de contêiner com o nome escolhido.

O procedimento confere SHA-256 do arquivo, executa `pg_restore --exit-on-error` no destino novo e compara versão/estado das migrações, contagens críticas **e assinaturas MD5 de conteúdo ordenadas de forma independente** para empresas, usuários, produtos, vendas, auditoria, numeração fiscal e intenções fiscais, além de 11 tabelas do esquema. A comparação de conteúdo detecta valores divergentes mesmo quando o número de linhas coincide; ela complementa a integridade do arquivo SHA-256 e não é uma assinatura de autenticidade. Erros deixam o CI vermelho. Ao encerrar, remove somente o contêiner **que ele mesmo criou** e o backup temporário. Não executa `DROP DATABASE` ou `DROP SCHEMA` nem acessa servidores reais.

Esse teste complementa o PR #69 (outro banco no mesmo contêiner). Nenhum deles substitui uma restauração em outro computador Windows físico, backup externo criptografado, ensaio de queda de energia ou atualização do servidor de produção. Documentos fiscais restaurados não são notas autorizadas pela SEFAZ.
