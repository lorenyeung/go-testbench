create database orchestrate;
create table users(user_id serial PRIMARY KEY, username VARCHAR (50) UNIQUE NOT NULL, email VARCHAR (50) UNIQUE NOT NULL, admin boolean, created_on TIMESTAMP NOT NULL, last_login TIMESTAMP);
create table containers(id serial PRIMARY KEY, owner VARCHAR (50) NOT NULL, editable boolean, deletable boolean, editors VARCHAR (50), cont_id VARCHAR (100), status VARCHAR (15), product VARCHAR(40), ports int);
create table versions(product VARCHAR (25), version VARCHAR (15));

/* TODO need to insert admin user to prevent database showing up/being deletable */