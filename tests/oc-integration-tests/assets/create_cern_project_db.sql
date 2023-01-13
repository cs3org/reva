DROP DATABASE IF EXISTS eosspaces;
CREATE DATABASE eosspaces DEFAULT CHARACTER SET "utf8";

use eosspaces;

CREATE TABLE projectspaces (
	project_name  VARCHAR(50),
	eos_relative_path VARCHAR(50),
	project_owner     VARCHAR(50),

        PRIMARY KEY( project_name )
);


INSERT INTO projectspaces(project_name, eos_relative_path, project_owner) VALUES ('a-webeos-liveness', 'a/a-webeos-liveness', 'webeossp');
INSERT INTO projectspaces(project_name, eos_relative_path, project_owner) VALUES ('a15-coatings-collab', 'a/a15-coatings-collab', 'a15cern');
INSERT INTO projectspaces(project_name, eos_relative_path, project_owner) VALUES ('abpdata', 'a/abpdata', 'abpdata');
INSERT INTO projectspaces(project_name, eos_relative_path, project_owner) VALUES ('abtua9', 'a/abtua9', 'abtua9');

