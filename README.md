# Upasham Plus Admin

A connected website administration platform for Upasham Plus. The Go API owns authentication, doctor profiles, structured site content, and image uploads. The React dashboard and public preview both read the same SQLite-backed API, so published edits are visible immediately.

The full page CMS is seeded from the current Upasham Plus public experience and covers Home, Our Work, Impact, Media, Technology, and Privacy. Administrators can create additional pages and build them from ordered, reusable sections.

## Stack

- Go `net/http` API with secure, expiring, HTTP-only cookie sessions
- SQLite (WAL mode) for admins, sessions, doctors, and site content
- bcrypt password hashing
- React + Vite admin interface
- Local image library with validated type and 5 MB upload limit

## Run locally

```powershell
cd web
npm install
npm run build
cd ..
$env:GOCACHE="$PWD\.cache\go-build"
$env:GOPATH="$PWD\.cache\go"
go mod download
go run .
```

Open `http://127.0.0.1:8080`. The connected public-site preview is at `/preview`.

Before the database is first created, set a unique initial credential. The
server refuses to create an administrator with a missing or short password:

```powershell
$env:ADMIN_EMAIL="owner@example.com"
$env:ADMIN_PASSWORD="replace-with-a-unique-password-of-12-or-more-characters"
$env:COOKIE_SECURE="true"
```

For local HTTP development, omit `COOKIE_SECURE`; enable it when the application
is served over HTTPS.

The database is created at `data/upasham.db` and uploaded images are stored under `data/uploads`.

## API

Public:

- `GET /api/public/site` — all published content and doctors
- `GET /api/public/doctors` — published doctor directory
- `GET /api/public/cms` — all published pages, sections, and repeatable items

Authenticated:

- `GET|POST /api/admin/doctors`
- `PUT|DELETE /api/admin/doctors/{id}`
- `GET|POST /api/admin/content`
- `PUT|DELETE /api/admin/content/{key}`
- `POST /api/admin/upload`
- `GET /api/admin/media` — complete searchable asset inventory
- `PUT /api/admin/media/{id}` — update accessible image description
- `DELETE /api/admin/media/{id}` — safely remove uploaded assets
- `GET|POST /api/admin/pages`
- `PUT|DELETE /api/admin/pages/{id}`
- `POST /api/admin/pages/{id}/sections`
- `PUT|DELETE /api/admin/sections/{id}`
- `POST /api/admin/sections/{id}/items`
- `PUT|DELETE /api/admin/items/{id}`

The original Upasham frontend can consume `GET /api/public/site` for globals/doctors and `GET /api/public/cms` for the complete page tree. Each published page contains ordered sections; each section can contain ordered repeatable items for cards, steps, FAQs, media, locations, press stories, statistics, and galleries.

## CMS capabilities

- Create, edit, draft, publish, order, and delete pages
- Edit SEO title, SEO description, navigation label, and route slug
- Add, edit, hide, order, and delete page sections
- Section types for hero, rich text, feature, cards, steps, FAQs, doctors, gallery, articles, videos, profiles, statistics, collections, and contact blocks
- Add repeatable content items with title, label, description, image, link, visibility, and order
- Manage doctors independently and render the published doctor directory in any doctor section
- Manage global brand, homepage fallback, services, testimonials, contact details, and footer content
- Upload images once and use their stable URL in any page, section, item, or doctor profile
- Browse all 57 existing site images alongside new uploads, copy URLs, and maintain alt text
- Add or remove reusable global fields without changing application code
