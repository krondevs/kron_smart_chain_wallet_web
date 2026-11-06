# Migraciones

El MVP usa `gorm.AutoMigrate` al iniciar (`internal/db/db.go`) para crear y
actualizar el esquema inicial. Esto es suficiente para una sola instancia con
SQLite.

Para producción con cambios de esquema frecuentes se recomienda reemplazarlo por
migraciones versionadas (`goose`, `golang-migrate` o similar), con backups antes
de cada despliegue. Este directorio queda reservado para esos archivos.
