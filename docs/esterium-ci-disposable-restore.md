# Comprovação automatizada de restauração PostgreSQL — Esterium ERP

O job `integration` executa `scripts/ci-postgres-disposable-restore.sh` depois de aplicar migrações e gerar os dados sintéticos de demonstração no PostgreSQL efêmero do GitHub Actions.

## O que o teste realmente faz

1. Confirma execução exclusiva em GitHub Actions e que o banco fonte é exatamente `sistemaemgo`.
2. Recusa qualquer banco com nome `sistemaemgo_restore_ci` já existente. **Não executa DROP** em bancos ou schemas.
3. Executa `pg_dump` custom-format, valida arquivo não vazio, verifica hash SHA-256 e inspeciona o catálogo do arquivo via `pg_restore --list`.
4. Cria um segundo banco novo com nome fixo e explícito `sistemaemgo_restore_ci`, restaura todo o dump com `pg_restore --exit-on-error`.
5. Compara a versão e o estado limpo das migrações, totais de empresas, usuários, produtos, vendas, auditoria, sequência fiscal e intenções fiscais. Confirma a existência de 11 tabelas críticas.
6. Emite `PASS` apenas se todas as comparações forem iguais. Arquivo temporário é eliminado, e os contêineres/bancos efêmeros do runner serão descartados no encerramento do job.

Nenhuma alteração é aplicada a banco de loja, servidor conectado do usuário ou repositório `main`. A restauração verifica que os dados de exemplo sobreviveram à cópia integral **dentro da mesma instância PostgreSQL de CI**; isso é diferente de provar restauração em outro Windows, criptografia de backup, isolamento de servidor, atualização e recuperação após falha de energia.

O script é propositalmente exclusivo do GitHub CI. A homologação do ambiente de loja deve usar um procedimento independente, controlado por operador autorizado, com os próprios arquivos de backup e outro computador de testes. É proibido testar restauração destrutiva em banco de produção.

**Emissão fiscal:** histórico restaurado não equivale a documento SEFAZ autorizado. #32/#33 seguem bloqueados.
