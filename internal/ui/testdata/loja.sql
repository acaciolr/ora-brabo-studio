-- Esquema de demonstração LOJA (prints do README e testes ao vivo): clientes,
-- produtos, pedidos e itens com dados gerados. Rode como DBA no PDB de teste.
-- by • DBA BRABO
begin
  for u in (select username from dba_users where username = 'LOJA') loop
    execute immediate 'drop user loja cascade';
  end loop;
end;
/
create user loja identified by "Loja#2026x" default tablespace users quota unlimited on users;
grant connect, resource, create view, dba to loja;
create table loja.departamentos (
  id_depto   number primary key,
  nome       varchar2(40) not null,
  cidade     varchar2(40)
);
create table loja.funcionarios (
  id_func    number primary key,
  nome       varchar2(60) not null,
  cargo      varchar2(40),
  salario    number(10,2),
  admissao   date,
  id_depto   number references loja.departamentos
);
create table loja.clientes (
  id_cliente number primary key,
  nome       varchar2(80) not null,
  email      varchar2(120),
  cidade     varchar2(40),
  uf         char(2),
  criado_em  date default sysdate
);
create table loja.produtos (
  id_produto number primary key,
  nome       varchar2(80) not null,
  categoria  varchar2(30),
  preco      number(10,2)
);
create table loja.pedidos (
  id_pedido  number primary key,
  id_cliente number not null references loja.clientes,
  id_func    number references loja.funcionarios,
  data_ped   date not null,
  status     varchar2(12)
);
create table loja.itens_pedido (
  id_pedido  number references loja.pedidos,
  item       number,
  id_produto number references loja.produtos,
  qtd        number,
  preco      number(10,2),
  constraint itens_pk primary key (id_pedido, item)
);
insert into loja.departamentos values (10, 'Vendas', 'São Paulo');
insert into loja.departamentos values (20, 'Financeiro', 'São Paulo');
insert into loja.departamentos values (30, 'Logística', 'Campinas');
insert into loja.departamentos values (40, 'TI', 'Belo Horizonte');
insert into loja.departamentos values (50, 'Atendimento', 'Recife');
declare
  type t is table of varchar2(40);
  pn t := t('Ana', 'Bruno', 'Carla', 'Diego', 'Eduarda', 'Felipe', 'Gabriela', 'Henrique', 'Isabela', 'João',
            'Larissa', 'Marcos', 'Natália', 'Otávio', 'Paula', 'Rafael', 'Sabrina', 'Thiago', 'Vanessa', 'Wagner');
  sn t := t('Silva', 'Santos', 'Oliveira', 'Souza', 'Lima', 'Pereira', 'Costa', 'Ferreira', 'Almeida', 'Ribeiro',
            'Carvalho', 'Gomes', 'Martins', 'Rocha', 'Barbosa', 'Araújo', 'Melo', 'Cardoso', 'Teixeira', 'Moreira');
  cid t := t('São Paulo', 'Rio de Janeiro', 'Belo Horizonte', 'Curitiba', 'Porto Alegre', 'Salvador', 'Recife', 'Fortaleza', 'Brasília', 'Campinas');
  uf t := t('SP', 'RJ', 'MG', 'PR', 'RS', 'BA', 'PE', 'CE', 'DF', 'SP');
  cg t := t('Gerente', 'Vendedor', 'Vendedor', 'Analista', 'Assistente', 'Coordenador');
  cat t := t('Informática', 'Celulares', 'Eletrodomésticos', 'Livros', 'Games', 'Casa', 'Esporte', 'Moda');
  pr t := t('Notebook', 'Monitor', 'Teclado', 'Mouse', 'Smartphone', 'Fone', 'Geladeira', 'Micro-ondas', 'Livro', 'Console',
            'Cadeira', 'Mesa', 'Bicicleta', 'Tênis', 'Camiseta', 'Mochila', 'Cafeteira', 'Ventilador', 'Câmera', 'Tablet');
  n number;
begin
  dbms_random.seed(2026);
  for i in 1 .. 60 loop
    insert into loja.funcionarios values (i, pn(mod(i * 7, 20) + 1) || ' ' || sn(mod(i * 3, 20) + 1),
      cg(mod(i, 6) + 1), round(3500 + dbms_random.value(0, 14000), 2), date '2015-01-01' + trunc(dbms_random.value(0, 3800)),
      (mod(i, 5) + 1) * 10);
  end loop;
  for i in 1 .. 2000 loop
    n := mod(i * 13, 10) + 1;
    insert into loja.clientes values (i, pn(mod(i, 20) + 1) || ' ' || sn(mod(i * 7, 20) + 1) || ' ' || sn(mod(i * 11, 20) + 1),
      lower(pn(mod(i, 20) + 1)) || '.' || i || '@exemplo.com.br', cid(n), uf(n), date '2022-01-01' + trunc(dbms_random.value(0, 1400)));
  end loop;
  for i in 1 .. 200 loop
    insert into loja.produtos values (i, pr(mod(i, 20) + 1) || ' ' || chr(65 + mod(i, 26)) || to_char(100 + i),
      cat(mod(i, 8) + 1), round(dbms_random.value(19, 7999), 2));
  end loop;
  for i in 1 .. 30000 loop
    insert into loja.pedidos values (i, trunc(dbms_random.value(1, 2001)), trunc(dbms_random.value(1, 61)),
      date '2024-01-01' + dbms_random.value(0, 680),
      case when dbms_random.value < 0.85 then 'ENTREGUE' when dbms_random.value < 0.6 then 'EM ROTA' else 'CANCELADO' end);
    for j in 1 .. trunc(dbms_random.value(1, 5)) loop
      n := trunc(dbms_random.value(1, 201));
      insert into loja.itens_pedido select i, j, n, trunc(dbms_random.value(1, 4)), preco from loja.produtos where id_produto = n;
    end loop;
  end loop;
  commit;
end;
/
create index loja.pedidos_cli on loja.pedidos (id_cliente);
create index loja.pedidos_data on loja.pedidos (data_ped);
create index loja.itens_prod on loja.itens_pedido (id_produto);
create or replace view loja.vendas_mes as
  select trunc(p.data_ped, 'MM') mes, count(distinct p.id_pedido) pedidos, sum(i.qtd * i.preco) total
    from loja.pedidos p join loja.itens_pedido i on i.id_pedido = p.id_pedido
   where p.status <> 'CANCELADO'
   group by trunc(p.data_ped, 'MM');
create sequence loja.pedidos_seq start with 30001;
create or replace package loja.vendas as
  function total_cliente(p_cliente number) return number;
  procedure fechar_mes(p_mes date);
end;
/
create or replace package body loja.vendas as
  function total_cliente(p_cliente number) return number is
    v number;
  begin
    select nvl(sum(i.qtd * i.preco), 0) into v
      from loja.pedidos p join loja.itens_pedido i on i.id_pedido = p.id_pedido
     where p.id_cliente = p_cliente and p.status <> 'CANCELADO';
    return v;
  end;
  procedure fechar_mes(p_mes date) is
  begin
    update loja.pedidos set status = 'ENTREGUE'
     where status = 'EM ROTA' and data_ped < add_months(trunc(p_mes, 'MM'), 1);
  end;
end;
/
begin
  dbms_stats.gather_schema_stats('LOJA');
end;
/
