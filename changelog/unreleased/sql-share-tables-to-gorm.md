Enhancement: Convert SQL tables to gorm, corresponding driver, and tests

- Conversion of the SQL tables to a GORM model, IDs are unique across public links, normal shares, and OCM shares.
   - Some refactoring of the OCM tables (protocols and access methods)
- Corresponding SQL driver for access has been implemented using GORM
- Tests with basic coverage have been implemented.

https://github.com/cs3org/reva/pull/5381