# Esterium Local — serviço Windows (prova L2)

O executável Go agora identifica quando foi iniciado pelo Gerenciador de Serviços do Windows (SCM), informa os estados de início/execução e recebe comandos de **Parar** e **Desligar** para solicitar um encerramento controlado do servidor HTTP.

A mesma aplicação continua funcionando no console Windows e no Linux sem depender do SCM. Este suporte **não instala nem registra o serviço automaticamente** e não modifica a base PostgreSQL.

## Restrições antes de instalar na loja

- A conta do serviço deve ser dedicada, sem privilégios administrativos para o dia a dia, e com acesso apenas às pastas do programa, logs e arquivos necessários.
- O arquivo de configuração e os segredos devem ser provisionados com permissões exclusivas; não colocar a senha do banco, JWT ou token no comando `sc.exe`, em atalhos ou em arquivos do repositório.
- O serviço deve iniciar somente após PostgreSQL e um backend seguro para as sessões de autenticação estarem disponíveis. O Redis permanece obrigatório em modo de produção até uma alternativa comprovadamente equivalente.
- A pasta compilada do frontend deve ser indicada por `ESTERIUM_WEB_DIR`, com `index.html` presente. Configure o endereço do servidor HTTP e o TLS conforme o projeto de instalação.
- Evite liberar a porta de administração para a Internet. Terminais na LAN só devem acessar HTTPS validado; banco de dados e armazenamento de sessões não devem ser expostos.
- Proteger desligamentos com pendências offline e implementar backup externo/restauração antes de considerar um ambiente operacional.

## Aceite ainda necessário

Compilar o binário x64 no CI não demonstra que ele instala, reinicia após reboot, informa status correto no Windows 10/11, responde a Parar/Desligar com solicitações em andamento ou se recupera após falha de energia. Esses testes precisam ser executados em uma VM Windows limpa com serviços e dados **descartáveis**. Não confundir esta implementação de protocolo SCM com o instalador final.

A emissão de NFC-e/NF-e real permanece bloqueada por homologação e pela validação de protocolo SEFAZ.
