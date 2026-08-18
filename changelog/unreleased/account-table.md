Enhancement: add account table

Introduces the `account` table (guid, unique_identifier, subscription_status,
account_type, owner_id, created_at, updated_at), managed through gorm
AutoMigrate like the other sql-backed managers.

https://github.com/cs3org/reva/pull/5782
