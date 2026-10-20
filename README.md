# Previsix

🥐 Previsix est un projet simple de projection d'affluence pour le parc d'attraction français Parc Astérix. Son objectif est de produire des projections fiables basées sur différentes séries de données, afin d'estimer le plus précisément possible la fréquentation attendue aux différentes dates d'opération du parc. Projet personnel réalisé pour le fun :)

🇬🇧 Previsix is a simple attendance projection project for the French theme park Parc Astérix. Its aim is to produce reliable projections based on a series of useful data, in order to estimate as accurately as possible the expected attendance at the park's various operating dates. Personal project done for fun!

## Fonctionnement

Previsix tourne en continu et enchaîne plusieurs tâches planifiées :

| Tâche | Fréquence | Rôle |
|---|---|---|
| `syncCalendar` | 6 h | Récupère les jours et horaires d'ouverture (API officielle si configurée, sinon [ThemeParks.wiki](https://themeparks.wiki)), crée et met à jour les créneaux, désactive ceux retirés du calendrier. |
| `syncHolidays` | quotidienne | Récupère les vacances scolaires des zones A, B et C ([data.education.gouv.fr](https://data.education.gouv.fr)). |
| `collectWaitTimes` | 5 min | Enregistre les temps d'attente en direct de toutes les attractions pendant l'ouverture. |
| `learn` | 1 h | Résume les temps d'attente des créneaux terminés, puis réentraîne le modèle. |
| `queueForecasts` | 30 à 45 min | Choisit les créneaux à prévoir en priorité et lance leurs prévisions en parallèle. |

### Le modèle

Le modèle prévoit le **temps d'attente moyen** du parc sur un créneau (journée ou soirée événement), à partir de :

- le jour de la semaine ;
- les vacances scolaires par zone (la zone C, Île-de-France, pèse le plus) ;
- les jours fériés (calculés, Pâques compris) et les ponts ;
- les soirées événements (Peur sur le Parc…) ;
- l'amplitude d'ouverture, que le parc allonge les jours d'affluence ;
- la météo ([Open-Meteo](https://open-meteo.com)) à moins de 16 jours.

C'est une régression linéaire sur le logarithme du temps d'attente. Au départ, les coefficients sont une estimation experte. Une fois assez de jours observés (`model.minTrainingDays`), le modèle est ajusté sur les temps d'attente réellement mesurés par une régression ridge **tirée vers ces coefficients de départ** : avec peu de données il reste proche de l'estimation experte, puis s'en écarte à mesure que les observations s'accumulent. La courbe horaire de la journée est elle aussi apprise.

Chaque prévision donne :

- `avgWait` : le temps d'attente moyen attendu, en minutes ;
- `crowdScore` : un score d'affluence de 0 (parc vide) à 1 (bondé), sur une échelle logarithmique de 5 à 60 minutes ;
- `confidence` : un indice de confiance, qui baisse avec l'éloignement de la date, sans météo connue ou quand le modèle se trompe souvent ;
- `waitTimes` : le temps d'attente attendu heure par heure.

La priorité de prévision d'un créneau est `heures depuis la dernière prévision / (heures avant le créneau) ^ priorityExponent`. Les créneaux jamais prévus passent en premier, et les créneaux proches sont rafraîchis plus souvent que les lointains.

Après chaque créneau, la dernière prévision faite avant l'ouverture est comparée au temps d'attente observé, ce qui mesure la précision réelle du modèle (affichée dans l'interface et sur `/api/model`).

## Lancer le projet

Avec Docker :

```sh
docker compose up -d
```

L'interface est alors disponible sur http://localhost:3000.

Sans Docker (Go 1.25+ et MongoDB requis) :

```sh
cp config.example.yml config.yml   # optionnel, tout a une valeur par défaut
go run . --config config.yml
```

Chaque réglage peut aussi être passé en variable d'environnement : `PREVISIX_<SECTION>_<CLÉ>`, par exemple `PREVISIX_DATABASE_URI` ou `PREVISIX_FORECASTS_INTERVAL_MIN`. Voir [`config.example.yml`](config.example.yml) pour la liste des réglages.

## API

| Route | Description |
|---|---|
| `GET /api/health` | État du serveur et de la base de données. |
| `GET /api/days?from=AAAA-MM-JJ&to=AAAA-MM-JJ` | Créneaux d'ouverture avec leur dernière prévision et, une fois passés, leur observation. Par défaut, les 120 prochains jours. |
| `GET /api/days/AAAA-MM-JJ` | Créneaux d'une journée avec l'historique de leurs prévisions. |
| `GET /api/live` | Derniers temps d'attente relevés. |
| `GET /api/model` | Coefficients du modèle, nombre de créneaux observés et précision mesurée. |

## Développement

```sh
go vet ./...
go test ./...
# Tests d'intégration avec MongoDB :
PREVISIX_TEST_MONGO_URI=mongodb://127.0.0.1:27017 go test ./...
```

## Sources de données

Temps d'attente et horaires : [ThemeParks.wiki](https://themeparks.wiki) · Météo : [Open-Meteo](https://open-meteo.com) · Vacances scolaires : [data.education.gouv.fr](https://data.education.gouv.fr) (Licence Ouverte). Projet non affilié au Parc Astérix.
