# Solar Tracker

## Why name Solar?

Archive Team uses [ArchiveTeam/Universal-tracker](https://github.com/ArchiveTeam/Universal-tracker).
The solar system is insignificant compared to the universe. (I know, it's "Universal", not "Universe")
Compared to ArchiveTeam, we don't have a lot of projects to track, so... "Solar Tracker" here is!

## Tracker Configuration

```bash
MONGODB_URI=mongodb://aa:bb@localhost:27017,s2:27017,s3:27017/?replicaSet=rs0 # mongodb uri
GIN_MODE=release # set to release to disable debug mode
PORT=8080 # port to listen, default 8080
```

## Project configuration

[projects.go](./projects.go)