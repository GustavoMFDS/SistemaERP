# Esterium Local — teste de restauração entre instâncias independentes

O job de integração cria um backup custom-format do banco PostgreSQL **descartável do CI** e o restaura em um **segundo servidor PostgreSQL 16**, não apenas em outro banco da mesma instância.

Essa segunda instância usa contêiner próprio, sem volume compartilhado com a origem, sem rede (`--network none`) e sem portas publicadas. O teste exige runner GitHub hospedado, nome exclusivo por execução, fonte com nome exato `sistemaemgo`, migrações não pendentes e inexistência de contêiner com o nome escolhido.

O procedimento confere SHA-256 do arquivo, executa `pg_restore --exit-on-error` no destino novo e compara versão/estado das migrações, contagens críticas **e assinaturas MD5 de conteúdo ordenadas de forma independente** para empresas, usuários, produtos, vendas, auditoria, numeração fiscal e intenções fiscais, além de 11 tabelas do esquema. A comparação de conteúdo detecta valores divergentes mesmo quando o número de linhas coincide; ela complementa a integridade do arquivo SHA-256 e não é uma assinatura de autenticidade. Erros deixam o CI vermelho. Ao encerrar, remove somente o contêiner **que ele mesmo criou** e o backup temporário. Não executa `DROP DATABASE` ou `DROP SCHEMA` nem acessa servidores reais.

Esse teste complementa o PR #69 (outro banco no mesmo contêiner). Nenhum deles substitui uma restauração em outro computador Windows físico, backup externo criptografado, ensaio de queda de energia ou atualização do servidor de produção. Documentos fiscais restaurados não são notas autorizadas pela SEFAZ.

## Evidência adicional em Windows

O teste `test-esterium-postgres-real-tls-ci.ps1` agora cria **um segundo cluster PostgreSQL 16 do zero** em uma pasta temporária distinta no mesmo runner Windows. Ele escuta exclusivamente em `127.0.0.1`, usa porta diferente da origem e autenticação SCRAM, e não reutiliza arquivos, volumes, roles, dados ou processo do banco original. O banco `esterium_recovery_ci` é criado somente nesse cluster temporário e recebe o arquivo que passou pela conferência SHA-256 e `pg_restore --list`.

O teste executa `pg_restore --exit-on-error`, compara estado das migrações, resumo criptográfico de registros fictícios de teste, contagens críticas de empresas, usuários, produtos, vendas e registros fiscais, e verifica a existência de 11 tabelas essenciais. Depois ainda confirma que o banco de origem não foi alterado. A limpeza encerra somente as instâncias iniciadas pelo próprio teste e apaga a pasta isolada do runner. Nenhuma restauração é executada sobre dados reais.

**Limite:** são duas instâncias independentes no **mesmo computador virtual Windows descartável**, não dois computadores físicos. Isso acrescenta cobertura de ferramentas e procedimentos Windows ao teste Linux entre instâncias, mas **não** comprova conectividade entre máquinas, cópia SMB de verdade, restauração de imagens de produto ou operação após falha física. Esses ensaios continuam pendentes para a homologação do instalador e da recuperação do Esterium.

## Imagens dos produtos no backup

No Esterium, fotos e miniaturas dos produtos são gravadas na tabela
`product_images` do próprio PostgreSQL (campos binários `image_data` e
`thumb_data`). Não existe pasta separada de fotos para copiar no procedimento
atual; um `pg_dump` completo e restaurável deve preservar **também os bytes
das imagens, as legendas e a associação à empresa e ao produto**.

A verificação Windows agora grava uma imagem e uma miniatura **sintéticas**
vinculadas a produto descartável antes do teste de queda forçada; depois
restaura o backup em outro cluster e compara a assinatura do conteúdo
binário e da legenda, além de exigir a presença de `product_images` no
arquivo. O verificador externo recusa backup sem a entrada `TABLE DATA`
dessa tabela. Isso testa integridade de bytes e referências, **não**
a validade visual do JPEG, nem restauração em dois computadores físicos.
