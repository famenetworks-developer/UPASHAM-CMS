package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type app struct{ db *sql.DB }
type doctor struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Photo          string `json:"photo"`
	Specialty      string `json:"specialty"`
	Qualifications string `json:"qualifications"`
	Experience     string `json:"experience"`
	Bio            string `json:"bio"`
	Availability   string `json:"availability"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`
	Organization   string `json:"organization"`
	Featured       bool   `json:"featured"`
	SortOrder      int    `json:"sort_order"`
	Published      bool   `json:"published"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func main() {
	if err := os.MkdirAll("data/uploads", 0755); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", "data/upasham.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		log.Fatal(err)
	}
	a := &app{db: db}
	if err := a.migrate(); err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.HandleFunc("GET /api/auth/me", a.auth(a.me))
	mux.HandleFunc("GET /api/public/site", a.publicSite)
	mux.HandleFunc("GET /api/public/doctors", a.publicDoctors)
	mux.HandleFunc("GET /api/admin/doctors", a.auth(a.listDoctors))
	mux.HandleFunc("POST /api/admin/doctors", a.auth(a.createDoctor))
	mux.HandleFunc("PUT /api/admin/doctors/{id}", a.auth(a.updateDoctor))
	mux.HandleFunc("DELETE /api/admin/doctors/{id}", a.auth(a.deleteDoctor))
	mux.HandleFunc("GET /api/admin/content", a.auth(a.listContent))
	mux.HandleFunc("POST /api/admin/content", a.auth(a.createContent))
	mux.HandleFunc("PUT /api/admin/content/{key}", a.auth(a.updateContent))
	mux.HandleFunc("DELETE /api/admin/content/{key}", a.auth(a.deleteContent))
	mux.HandleFunc("POST /api/admin/upload", a.auth(a.upload))
	mux.HandleFunc("GET /api/admin/media", a.auth(a.listMedia))
	mux.HandleFunc("PUT /api/admin/media/{id}", a.auth(a.updateMedia))
	mux.HandleFunc("DELETE /api/admin/media/{id}", a.auth(a.deleteMedia))
	mux.HandleFunc("GET /api/public/cms", a.publicCMS)
	mux.HandleFunc("GET /api/admin/pages", a.auth(a.listPages))
	mux.HandleFunc("POST /api/admin/pages", a.auth(a.createPage))
	mux.HandleFunc("PUT /api/admin/pages/{id}", a.auth(a.updatePage))
	mux.HandleFunc("DELETE /api/admin/pages/{id}", a.auth(a.deletePage))
	mux.HandleFunc("POST /api/admin/pages/{id}/sections", a.auth(a.createSection))
	mux.HandleFunc("PUT /api/admin/sections/{id}", a.auth(a.updateSection))
	mux.HandleFunc("DELETE /api/admin/sections/{id}", a.auth(a.deleteSection))
	mux.HandleFunc("POST /api/admin/sections/{id}/items", a.auth(a.createItem))
	mux.HandleFunc("PUT /api/admin/items/{id}", a.auth(a.updateItem))
	mux.HandleFunc("DELETE /api/admin/items/{id}", a.auth(a.deleteItem))
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("data/uploads"))))
	mux.HandleFunc("/", spa)
	port := env("PORT", "8080")
	log.Printf("Upasham admin running at http://127.0.0.1:%s", port)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, security(mux)))
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://127.0.0.1:5174" || origin == "http://localhost:5174" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
func (a *app) migrate() error {
	schema := `CREATE TABLE IF NOT EXISTS admins(id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions(token TEXT PRIMARY KEY, admin_id INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE, expires_at DATETIME NOT NULL);
CREATE TABLE IF NOT EXISTS doctors(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,slug TEXT NOT NULL UNIQUE,photo TEXT NOT NULL DEFAULT '',specialty TEXT NOT NULL DEFAULT '',qualifications TEXT NOT NULL DEFAULT '',experience TEXT NOT NULL DEFAULT '',bio TEXT NOT NULL DEFAULT '',availability TEXT NOT NULL DEFAULT '',phone TEXT NOT NULL DEFAULT '',email TEXT NOT NULL DEFAULT '',organization TEXT NOT NULL DEFAULT '',featured INTEGER NOT NULL DEFAULT 0,sort_order INTEGER NOT NULL DEFAULT 0,published INTEGER NOT NULL DEFAULT 1,created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE IF NOT EXISTS content(key TEXT PRIMARY KEY,label TEXT NOT NULL,group_name TEXT NOT NULL,value TEXT NOT NULL,type TEXT NOT NULL DEFAULT 'text',updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);`
	schema += `CREATE TABLE IF NOT EXISTS pages(id INTEGER PRIMARY KEY AUTOINCREMENT,slug TEXT NOT NULL UNIQUE,title TEXT NOT NULL,nav_label TEXT NOT NULL DEFAULT '',description TEXT NOT NULL DEFAULT '',seo_title TEXT NOT NULL DEFAULT '',seo_description TEXT NOT NULL DEFAULT '',status TEXT NOT NULL DEFAULT 'published',sort_order INTEGER NOT NULL DEFAULT 0,created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE IF NOT EXISTS sections(id INTEGER PRIMARY KEY AUTOINCREMENT,page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,section_key TEXT NOT NULL,kind TEXT NOT NULL DEFAULT 'content',label TEXT NOT NULL,eyebrow TEXT NOT NULL DEFAULT '',heading TEXT NOT NULL DEFAULT '',body TEXT NOT NULL DEFAULT '',media_url TEXT NOT NULL DEFAULT '',cta_label TEXT NOT NULL DEFAULT '',cta_url TEXT NOT NULL DEFAULT '',data_json TEXT NOT NULL DEFAULT '{}',visible INTEGER NOT NULL DEFAULT 1,sort_order INTEGER NOT NULL DEFAULT 0,created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(page_id,section_key));
CREATE TABLE IF NOT EXISTS section_items(id INTEGER PRIMARY KEY AUTOINCREMENT,section_id INTEGER NOT NULL REFERENCES sections(id) ON DELETE CASCADE,title TEXT NOT NULL DEFAULT '',subtitle TEXT NOT NULL DEFAULT '',body TEXT NOT NULL DEFAULT '',image_url TEXT NOT NULL DEFAULT '',link_label TEXT NOT NULL DEFAULT '',link_url TEXT NOT NULL DEFAULT '',meta_json TEXT NOT NULL DEFAULT '{}',sort_order INTEGER NOT NULL DEFAULT 0,published INTEGER NOT NULL DEFAULT 1,created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);`
	schema += `CREATE TABLE IF NOT EXISTS media_assets(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,url TEXT NOT NULL UNIQUE,alt_text TEXT NOT NULL DEFAULT '',mime_type TEXT NOT NULL DEFAULT '',size_bytes INTEGER NOT NULL DEFAULT 0,deletable INTEGER NOT NULL DEFAULT 0,created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);`
	schema += `CREATE TABLE IF NOT EXISTS app_migrations(name TEXT PRIMARY KEY,applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);`
	if _, err := a.db.Exec(schema); err != nil {
		return err
	}
	var n int
	a.db.QueryRow("SELECT COUNT(*) FROM admins").Scan(&n)
	if n == 0 {
		password := os.Getenv("ADMIN_PASSWORD")
		if len(password) < 12 {
			return errors.New("ADMIN_PASSWORD must be set to at least 12 characters before first startup")
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		_, err := a.db.Exec("INSERT INTO admins(name,email,password_hash) VALUES(?,?,?)", "Upasham Administrator", env("ADMIN_EMAIL", "admin@upashamplus.com"), hash)
		if err != nil {
			return err
		}
	}
	contents := []struct{ k, l, g, v, t string }{
		{"brand.name", "Brand name", "Brand", "Upasham Plus", "text"}, {"brand.tagline", "Brand tagline", "Brand", "Healthcare that reaches home", "text"}, {"brand.logo", "Logo", "Brand", "/brand-logo.png", "image"},
		{"home.eyebrow", "Hero eyebrow", "Homepage", "Connected care across Bangladesh", "text"}, {"home.title", "Hero title", "Homepage", "Doctors from far, not so far.", "text"}, {"home.description", "Hero description", "Homepage", "BMDC-certified doctors, trusted technology, and community care—working together so quality healthcare feels closer.", "textarea"}, {"home.hero_image", "Hero image", "Homepage", "", "image"}, {"home.primary_cta", "Primary button label", "Homepage", "Book a consultation", "text"}, {"home.secondary_cta", "Secondary button label", "Homepage", "Meet our doctors", "text"},
		{"about.title", "Section title", "About", "Care built around people, not distance.", "text"}, {"about.body", "About description", "About", "Upasham Plus connects patients, doctors, pharmacies and community health partners through one thoughtful care journey.", "textarea"},
		{"services.title", "Section title", "Services", "Everything needed for connected care", "text"}, {"services.items", "Service cards", "Services", `[{"title":"Doctor consultation","description":"Speak with qualified doctors from wherever you are."},{"title":"Digital prescription","description":"Receive clear, secure prescriptions and follow-up guidance."},{"title":"Community health","description":"Access assisted care through trusted local partners."}]`, "json"},
		{"testimonials.title", "Section title", "Testimonials", "Care stories from our community", "text"}, {"testimonials.items", "Testimonials", "Testimonials", `[{"quote":"The doctor listened carefully and explained every step clearly.","name":"Upasham patient","role":"Sylhet"},{"quote":"We received support without travelling for hours.","name":"Community member","role":"Sunamganj"}]`, "json"},
		{"contact.title", "Section title", "Contact", "Let’s make care easier to reach", "text"}, {"contact.phone", "Phone", "Contact", "+880 1600-000000", "text"}, {"contact.email", "Email", "Contact", "care@upashamplus.com", "text"}, {"contact.address", "Address", "Contact", "Bangladesh", "textarea"}, {"footer.copyright", "Copyright", "Footer", "© 2026 Upasham Plus. All rights reserved.", "text"}, {"footer.links", "Footer links", "Footer", `[{"label":"Privacy","url":"/privacy"},{"label":"Media","url":"/media"},{"label":"Contact","url":"#contact"}]`, "json"},
	}
	for _, c := range contents {
		a.db.Exec("INSERT OR IGNORE INTO content(key,label,group_name,value,type) VALUES(?,?,?,?,?)", c.k, c.l, c.g, c.v, c.t)
	}
	if err := a.seedFullSiteContent(); err != nil {
		return err
	}
	var d int
	a.db.QueryRow("SELECT COUNT(*) FROM doctors").Scan(&d)
	if d == 0 {
		seeds := [][]any{
			{"Dr. Jony Chowdhury", "dr-jony-chowdhury", "/seed/dr-jony-chowdhury.jpg", "Cardiology & Critical Care", "PGT Cardiology (BMU) · CCD (BIRDEM)", "Senior medical advisor", "Experienced in cardiology, intensive care and internal medicine.", "Sun–Thu · 6:00 PM–10:00 PM", "", "", "Senior Advisor", 1, 1, 1},
			{"Dr. Shaila Sharmin Shahnewaz", "dr-shaila-sharmin", "/seed/dr-shaila-sharmin-shahnewaz-studio.png", "Physical Medicine & Rehabilitation", "MBBS · FCPS (Physical Medicine & Rehabilitation)", "Associate Professor", "Specialist consultant focused on recovery, mobility and rehabilitation.", "By appointment", "", "", "Shaheed Monsur Ali Medical College Hospital", 1, 2, 1},
			{"Dr. Sadia Rahman Shifa", "dr-sadia-rahman-shifa", "/seed/dr-sadia-rahman-shifa.png", "General Medicine", "MBBS", "Medical Officer", "Compassionate primary care and telemedicine physician.", "Sat–Thu · 9:00 AM–5:00 PM", "", "", "US-Bangla Medical College Hospital", 1, 3, 1},
		}
		for _, x := range seeds {
			a.db.Exec(`INSERT INTO doctors(name,slug,photo,specialty,qualifications,experience,bio,availability,phone,email,organization,featured,sort_order,published) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, x...)
		}
	}
	if err := a.applyDoctorRosterMigration(); err != nil {
		return err
	}
	if err := a.applyDoctorRosterV2(); err != nil {
		return err
	}
	if err := a.seedCMS(); err != nil {
		return err
	}
	if err := a.seedMedia(); err != nil {
		return err
	}
	return nil
}

func (a *app) seedFullSiteContent() error {
	b, err := os.ReadFile(filepath.Join("data", "full-site-content.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var entries []struct {
		Key   string `json:"key"`
		Label string `json:"label"`
		Group string `json:"group"`
		Value string `json:"value"`
		Type  string `json:"type"`
	}
	if err := json.Unmarshal(b, &entries); err != nil {
		return fmt.Errorf("decode full site content: %w", err)
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare("INSERT OR IGNORE INTO content(key,label,group_name,value,type) VALUES(?,?,?,?,?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if _, err = stmt.Exec(e.Key, e.Label, e.Group, e.Value, e.Type); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *app) applyDoctorRosterMigration() error {
	var done int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM app_migrations WHERE name='doctor_roster_live_v1'").Scan(&done); err != nil {
		return err
	}
	if done > 0 {
		return nil
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	type row struct {
		name, slug, photo, specialty, qualifications, experience, bio, availability, organization string
		order                                                                                     int
	}
	doctors := []row{
		{"Dr. Jony Chowdhury", "dr-jony-chowdhury", "/images/dr-jony-chowdhury.jpg", "Cardiology & Critical Care", "PGT Cardiology (BMU)\nCCD (BIRDEM)\nEx MO CCU (Ad-Din Hospital)\nEx MO Nephrology (National Kidney Foundation)\nEx MO Cardiology (National Heart Foundation)\nEx MO ICU (Shahbuddin Medical College Hospital)\nEx MO Medicine (United Hospital)", "Senior Advisor", "Senior medical advisor with experience across cardiology, critical care, nephrology and internal medicine.", "Contact Upasham Plus for availability", "Upasham Plus", 1},
		{"Dr. Shuvojit Paik", "dr-shuvojit-paik", "/images/dr-shuvojit-paik.jpg", "General Physician · Paediatrics & NICU", "M.B.B.S (Dhaka University)\nPGT in Suhrawardy Medical College Hospital\nPGT in Bangladesh Sishu Hospital\nCCD (BIRDEM)\nEx MO - Paediatrics & NICU (AMZ Hospital)\nEx MO - Nephrology (Shaheed Monsur Ali Medical College Hospital)\nEx MO - Neurology (Shikder Women's Medical College Hospital)\nSenior MO - Paediatrics and NICU (Shahabuddin Medical College Hospital)\nSenior MO - General Physician (Upasham Plus)", "Doctor", "General physician with extensive paediatrics, NICU, nephrology and neurology experience.", "Contact Upasham Plus for availability", "Upasham Plus", 2},
		{"Dr. Arshi Tasnim", "dr-arshi-tasnim", "/images/dr-arshi-tasnim.jpg", "General Medicine & Diagnostic Ultrasound", "MBBS\nDMU, CMU, BLST", "Doctor", "General physician with diagnostic ultrasound training.", "Contact Upasham Plus for availability", "Upasham Plus", 3},
		{"Dr. Sadia Rahman Shifa", "dr-sadia-rahman-shifa", "/images/dr-sadia-rahman-shifa.png", "General Medicine", "MBBS\nUS-Bangla Medical College Hospital", "Medical Officer", "Medical officer providing patient-centred general medicine and telemedicine care.", "Contact Upasham Plus for availability", "Upasham Plus", 4},
		{"Dr. Dipa Agarwala", "dr-dipa-agarwala", "/images/dr-dipa-agarwala.webp", "General Medicine", "MBBS", "Doctor", "General medicine doctor supporting accessible connected care.", "Contact Upasham Plus for availability", "Upasham Plus", 5},
	}
	for _, d := range doctors {
		_, err = tx.Exec(`INSERT INTO doctors(name,slug,photo,specialty,qualifications,experience,bio,availability,organization,featured,sort_order,published) VALUES(?,?,?,?,?,?,?,?,?,1,?,1) ON CONFLICT(slug) DO UPDATE SET name=excluded.name,photo=excluded.photo,specialty=excluded.specialty,qualifications=excluded.qualifications,experience=excluded.experience,bio=excluded.bio,availability=excluded.availability,organization=excluded.organization,featured=1,sort_order=excluded.sort_order,published=1,updated_at=CURRENT_TIMESTAMP`, d.name, d.slug, d.photo, d.specialty, d.qualifications, d.experience, d.bio, d.availability, d.organization, d.order)
		if err != nil {
			return err
		}
	}
	if _, err = tx.Exec("DELETE FROM doctors WHERE slug='dr-shaila-sharmin'"); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO app_migrations(name) VALUES('doctor_roster_live_v1')"); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *app) applyDoctorRosterV2() error {
	var done int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM app_migrations WHERE name='doctor_roster_source_v2'").Scan(&done); err != nil {
		return err
	}
	if done > 0 {
		return nil
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO doctors(name,slug,photo,specialty,qualifications,experience,bio,availability,organization,featured,sort_order,published)
		VALUES(?,?,?,?,?,?,?,?,?,1,3,1)
		ON CONFLICT(slug) DO UPDATE SET name=excluded.name,photo=excluded.photo,specialty=excluded.specialty,qualifications=excluded.qualifications,experience=excluded.experience,bio=excluded.bio,availability=excluded.availability,organization=excluded.organization,featured=1,sort_order=3,published=1,updated_at=CURRENT_TIMESTAMP`,
		"Dr. Shaila Sharmin Shahnewaz", "dr-shaila-sharmin-shahnewaz", "/images/dr-shaila-sharmin-shahnewaz-studio.png", "Physical Medicine & Rehabilitation", "MBBS\nFCPS (Physical Medicine & Rehabilitation)\nShaheed Monsur Ali Medical College Hospital, Dhaka", "Special consult doctor", "Associate Professor and specialist in Physical Medicine & Rehabilitation.", "Contact Upasham Plus for availability", "Associate Professor, Department of Physical Medicine & Rehabilitation")
	if err != nil {
		return err
	}
	updates := []struct {
		slug  string
		name  string
		order int
	}{
		{"dr-jony-chowdhury", "Dr. Jony Chowdhury", 1},
		{"dr-shuvojit-paik", "Dr. Shuvojit Paik", 2},
		{"dr-arshi-tasnim", "Dr. Arshi Tasnim", 4},
		{"dr-sadia-rahman-shifa", "Dr. Sadia Rahman Shifa", 5},
		{"dr-dipa-agarwala", "Dr.Dipa Agarwala", 6},
	}
	for _, d := range updates {
		if _, err = tx.Exec("UPDATE doctors SET name=?,sort_order=?,published=1,updated_at=CURRENT_TIMESTAMP WHERE slug=?", d.name, d.order, d.slug); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("INSERT INTO app_migrations(name) VALUES('doctor_roster_source_v2')"); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *app) seedMedia() error {
	root := filepath.Join("web", "public", "images")
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".svg": true}[ext] {
			return nil
		}
		rel, e := filepath.Rel(filepath.Join("web", "public"), path)
		if e != nil {
			return nil
		}
		info, _ := d.Info()
		var size int64
		if info != nil {
			size = info.Size()
		}
		_, e = a.db.Exec("INSERT OR IGNORE INTO media_assets(name,url,mime_type,size_bytes,deletable) VALUES(?,?,?,?,0)", d.Name(), "/"+filepath.ToSlash(rel), ext, size)
		return e
	})
}

func (a *app) seedCMS() error {
	var n int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM pages").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	type seedSection struct {
		k, kind, label, eye, head, body, media, cta, href string
		items                                             [][]string
	}
	type seedPage struct {
		slug, title, nav, desc string
		sections               []seedSection
	}
	pages := []seedPage{
		{"home", "Home", "Home", "Upasham Plus connected healthcare homepage", []seedSection{
			{"hero", "hero", "Hero", "BRINGING QUALITY HEALTHCARE CLOSER TO BANGLADESH", "Doctors from far, not so far.", "Connecting patients across Bangladesh with BMDC-certified doctors — by phone, on WhatsApp, or at a nearby Upasham Point.", "/images/hero/board-community.jpg", "Get a consultation", "#consultation", nil},
			{"care-channels", "cards", "Care channels", "GOOD CARE, YOUR WAY", "One team. Three ways to connect.", "Choose the way that works for you.", "", "", "", [][]string{{"Just a phone call", "No internet? No problem.", "Connect with care using a regular phone."}, {"Connect on WhatsApp", "Care, a message away.", "Chat with our care team and reach a doctor."}, {"Visit an Upasham Point", "A helping hand nearby.", "Get assisted care through a community partner."}}},
			{"clinical-team", "doctors", "Clinical team", "CLINICAL TEAM", "Doctors supporting patient care.", "Our doctors support patients with practical guidance, clear next steps, and consultations that feel close to home.", "", "Meet the clinical team", "#doctors", nil},
			{"technology", "feature", "Technology", "OUR TECHNOLOGY", "Connected technology. More personal care.", "From the first conversation to your prescription, a simpler experience for patients, doctors, and communities.", "/images/doctor-online.jpg", "Explore our technology", "/technology", nil},
			{"approach", "cards", "Our approach", "OUR APPROACH", "Care that meets people where they are.", "Bangladesh deserves a healthcare system that works for everyone, not just those with easy access to a hospital.", "", "", "", [][]string{{"Rural & Semi-Urban Access", "01", "Serving patients who face the greatest barriers to quality care."}, {"Workplace Wellness", "02", "Structured, reliable healthcare access for entire workforces."}, {"Mind and Body", "03", "General, specialist, and mental health consultations."}}},
			{"how-it-works", "steps", "How it works", "HOW IT WORKS", "Your doctor is just a few steps away.", "Let’s get you connected.", "", "", "", [][]string{{"Choose your channel", "01", "Call, message us on WhatsApp, or visit an Upasham Point."}, {"Tell us what you need", "02", "Share your concern with our team in your own words."}, {"Speak with a doctor", "03", "Connect with a certified doctor who listens and guides you."}, {"Keep your care close", "04", "Receive your prescription and advice for your next steps."}}},
			{"reach", "feature", "Our reach", "OUR REACH", "Rooted in Bangladesh. Built around people.", "From individual patients to large organizations, Upasham Plus is designed for the full breadth of Bangladesh's healthcare needs.", "/images/community-care-hero.png", "", "", nil},
			{"faq", "faq", "Frequently asked questions", "A LITTLE MORE CLARITY", "Good questions. Simple answers.", "Need a hand with something else? Our team is here to help.", "", "", "", [][]string{{"How do I book a consultation?", "", "Call or contact our team on WhatsApp. We will confirm the available doctor, timing, and consultation fee."}, {"Do I need a smartphone or internet?", "", "You can contact us using a regular phone. Video consultation requires internet."}, {"Can I speak to a doctor in Bengali?", "", "Yes. Our BMDC-certified doctors provide consultations in Bengali."}, {"Where can I find an Upasham Point?", "", "Contact our team to confirm the closest available partner care point."}, {"Can my organization partner with Upasham Plus?", "", "Yes. Email info@upashamplus.com to discuss your needs."}}},
			{"contact", "contact", "Contact", "LET'S TALK", "Closer to care. Starting with hello.", "Whether you are a patient, a partner organisation, or simply curious, our team is ready to help.", "", "Write to us", "mailto:info@upashamplus.com", nil},
		}},
		{"our-work", "Our Work", "Our Work", "Services, care channels, community partners and Upasham Points", []seedSection{{"hero", "hero", "Page hero", "OUR WORK", "Care beyond distance.", "Community-led healthcare across Bangladesh.", "/images/our-work-banner.jpg", "", "", nil}, {"work", "cards", "What we do", "WHAT WE DO", "One connected system. Many ways to care.", "Practical access designed around patients and partners.", "", "", "", [][]string{{"Upasham Points", "Community access", "Assisted healthcare through trusted local partners."}, {"Doctor consultation", "Clinical care", "Phone, audio and video consultations."}, {"Health screening", "Prevention", "Community and workplace screening programs."}, {"Health campaigns", "Awareness", "Useful health information where it matters."}, {"Workplace care", "Organizations", "Structured care access for teams."}}}, {"points", "collection", "Upasham Points", "UPASHAM POINTS", "Care close to the community.", "Manage locations, addresses, contacts and opening hours here.", "", "Partner with us", "mailto:info@upashamplus.com", nil}}},
		{"impact", "Impact", "Impact", "Leadership, story, campaign and impact reporting", []seedSection{{"hero", "hero", "Opening story", "OUR STORY", "Healthcare shaped by lived experience.", "Our mission, leadership and the communities that guide our work.", "/images/story-opening.jpg", "", "", nil}, {"founder", "profile", "Founder story", "LEADERSHIP", "Less distance. More possibility.", "The founding story and long-form leadership narrative.", "/images/leadership-ceo.jpg", "Read profile", "", nil}, {"program-lead", "profile", "Program lead", "COMMUNITY VOICE", "Built with communities, not just for them.", "Program leadership, quotation and biography.", "/images/leadership-cpo.png", "", "", nil}, {"campaigns", "gallery", "Campaign gallery", "IN THE FIELD", "Care in action.", "Community consultations, screenings and partner campaigns.", "", "", "", nil}, {"metrics", "stats", "Impact metrics", "OUR IMPACT", "Progress you can measure.", "Manage public reach, consultation and partner metrics.", "", "", "", [][]string{{"Divisions reached", "3", ""}, {"Community campaigns", "3", ""}, {"Connected care platform", "1", ""}}}}},
		{"media", "Media", "Media", "Press coverage, broadcasts and media gallery", []seedSection{{"hero", "hero", "Media hero", "MEDIA", "Stories worth sharing.", "Press coverage, broadcasts and updates from the field.", "/images/media/featured-reel.jpg", "", "", nil}, {"press", "articles", "Press articles", "PRESS COVERAGE", "Upasham Plus in the news.", "Create articles with publication, date, link and image.", "", "", "", nil}, {"broadcasts", "videos", "Broadcasts", "WATCH & LISTEN", "Conversations about connected care.", "Add interviews, reports and video features.", "", "", "", nil}}},
		{"technology", "Technology", "Technology", "Platform, clinical workflow, software and engineering partners", []seedSection{{"hero", "hero", "Technology hero", "OUR TECHNOLOGY", "Care powered by connection.", "A simpler workflow for patients, doctors, pharmacies, and community partners.", "/video/upasham-software-poster.jpg", "Explore the platform", "#software", nil}, {"software", "gallery", "Software showcase", "THE PLATFORM", "One workspace. A clearer care journey.", "Screens, captions, demonstrations and product explanations.", "", "", "", nil}, {"channels", "cards", "Care channels", "WAYS TO CONNECT", "Technology that adapts to people.", "Phone, WhatsApp and assisted consultations.", "", "", "", nil}, {"clinical-workspace", "feature", "Clinical workspace", "CLINICAL WORKSPACE", "Designed around the doctor-patient conversation.", "Manage workflow explanations and clinical platform visuals.", "/images/software/doctor-workspace.jpg", "", "", nil}, {"engineering", "profile", "Engineering leadership", "ENGINEERING", "Reliable systems, thoughtfully built.", "Technology leadership, partners and platform principles.", "/images/cto-ruhul-azim-rifath.jpg", "", "", nil}}},
		{"privacy", "Privacy", "Privacy", "Privacy policy and data handling information", []seedSection{{"policy", "rich-text", "Privacy policy", "PRIVACY", "Privacy at Upasham Plus.", "Manage the complete privacy policy, effective date, contact details and data-handling sections from this page.", "", "", "", nil}}},
	}
	for pi, p := range pages {
		res, err := a.db.Exec(`INSERT INTO pages(slug,title,nav_label,description,seo_title,seo_description,status,sort_order) VALUES(?,?,?,?,?,?,?,?)`, p.slug, p.title, p.nav, p.desc, p.title+" | Upasham Plus", p.desc, "published", pi)
		if err != nil {
			return err
		}
		pid, _ := res.LastInsertId()
		for si, s := range p.sections {
			res, err := a.db.Exec(`INSERT INTO sections(page_id,section_key,kind,label,eyebrow,heading,body,media_url,cta_label,cta_url,sort_order) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, pid, s.k, s.kind, s.label, s.eye, s.head, s.body, s.media, s.cta, s.href, si)
			if err != nil {
				return err
			}
			sid, _ := res.LastInsertId()
			for ii, item := range s.items {
				title, sub, body := "", "", ""
				if len(item) > 0 {
					title = item[0]
				}
				if len(item) > 1 {
					sub = item[1]
				}
				if len(item) > 2 {
					body = item[2]
				}
				a.db.Exec(`INSERT INTO section_items(section_id,title,subtitle,body,sort_order) VALUES(?,?,?,?,?)`, sid, title, sub, body, ii)
			}
		}
	}
	return nil
}

func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	jsonOut(w, status, map[string]string{"error": msg})
}
func decode(r *http.Request, v any) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	d.DisallowUnknownFields()
	return d.Decode(v)
}
func spa(w http.ResponseWriter, r *http.Request) {
	clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	clean = strings.TrimPrefix(clean, "admin/")
	if clean == "admin" {
		clean = "index.html"
	}
	if clean == "." || clean == "preview" {
		clean = "index.html"
	}
	full := filepath.Join("web/dist", clean)
	base, _ := filepath.Abs("web/dist")
	target, _ := filepath.Abs(full)
	if !strings.HasPrefix(target, base) || func() bool { _, e := os.Stat(target); return e != nil }() {
		full = filepath.Join("web/dist", "index.html")
	}
	http.ServeFile(w, r, full)
}
func token() string { b := make([]byte, 32); rand.Read(b); return hex.EncodeToString(b) }
func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var x struct{ Email, Password string }
	if decode(r, &x) != nil {
		fail(w, 400, "Invalid request")
		return
	}
	var id int64
	var name, hash string
	if a.db.QueryRow("SELECT id,name,password_hash FROM admins WHERE lower(email)=lower(?)", strings.TrimSpace(x.Email)).Scan(&id, &name, &hash) != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(x.Password)) != nil {
		fail(w, 401, "Email or password is incorrect")
		return
	}
	t := token()
	exp := time.Now().Add(12 * time.Hour)
	a.db.Exec("DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP")
	if _, err := a.db.Exec("INSERT INTO sessions(token,admin_id,expires_at) VALUES(?,?,?)", t, id, exp); err != nil {
		fail(w, 500, "Could not create session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "upasham_session", Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: env("COOKIE_SECURE", "") == "true", Expires: exp})
	jsonOut(w, 200, map[string]any{"name": name})
}
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("upasham_session"); e == nil {
		a.db.Exec("DELETE FROM sessions WHERE token=?", c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "upasham_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *app) admin(r *http.Request) (int64, string, error) {
	c, e := r.Cookie("upasham_session")
	if e != nil {
		return 0, "", e
	}
	var id int64
	var name string
	e = a.db.QueryRow(`SELECT a.id,a.name FROM sessions s JOIN admins a ON a.id=s.admin_id WHERE s.token=? AND s.expires_at>CURRENT_TIMESTAMP`, c.Value).Scan(&id, &name)
	return id, name, e
}
func (a *app) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, _, e := a.admin(r); e != nil {
			fail(w, 401, "Authentication required")
			return
		}
		next(w, r)
	}
}
func (a *app) me(w http.ResponseWriter, r *http.Request) {
	id, name, _ := a.admin(r)
	jsonOut(w, 200, map[string]any{"id": id, "name": name})
}

func scanDoctor(s interface{ Scan(...any) error }) (doctor, error) {
	var d doctor
	var f, p int
	e := s.Scan(&d.ID, &d.Name, &d.Slug, &d.Photo, &d.Specialty, &d.Qualifications, &d.Experience, &d.Bio, &d.Availability, &d.Phone, &d.Email, &d.Organization, &f, &d.SortOrder, &p, &d.CreatedAt, &d.UpdatedAt)
	d.Featured = f == 1
	d.Published = p == 1
	return d, e
}

const docCols = "id,name,slug,photo,specialty,qualifications,experience,bio,availability,phone,email,organization,featured,sort_order,published,created_at,updated_at"

func (a *app) doctors(where string, args ...any) ([]doctor, error) {
	rows, e := a.db.Query("SELECT "+docCols+" FROM doctors "+where, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []doctor{}
	for rows.Next() {
		d, e := scanDoctor(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (a *app) publicDoctors(w http.ResponseWriter, r *http.Request) {
	d, e := a.doctors("WHERE published=1 ORDER BY sort_order,id")
	if e != nil {
		fail(w, 500, "Could not load doctors")
		return
	}
	jsonOut(w, 200, d)
}
func (a *app) listDoctors(w http.ResponseWriter, r *http.Request) {
	d, e := a.doctors("ORDER BY sort_order,id")
	if e != nil {
		fail(w, 500, "Could not load doctors")
		return
	}
	jsonOut(w, 200, d)
}
func validateDoctor(d *doctor) error {
	d.Name = strings.TrimSpace(d.Name)
	d.Slug = strings.Trim(strings.ToLower(strings.ReplaceAll(d.Slug, " ", "-")), "-")
	if d.Name == "" {
		return errors.New("Doctor name is required")
	}
	if d.Slug == "" {
		return errors.New("URL slug is required")
	}
	if strings.ContainsAny(d.Slug, "/?#\\") {
		return errors.New("Slug may contain letters, numbers and hyphens only")
	}
	return nil
}
func (a *app) createDoctor(w http.ResponseWriter, r *http.Request) {
	var d doctor
	if decode(r, &d) != nil {
		fail(w, 400, "Invalid doctor data")
		return
	}
	if e := validateDoctor(&d); e != nil {
		fail(w, 422, e.Error())
		return
	}
	res, e := a.db.Exec(`INSERT INTO doctors(name,slug,photo,specialty,qualifications,experience,bio,availability,phone,email,organization,featured,sort_order,published) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, d.Name, d.Slug, d.Photo, d.Specialty, d.Qualifications, d.Experience, d.Bio, d.Availability, d.Phone, d.Email, d.Organization, d.Featured, d.SortOrder, d.Published)
	if e != nil {
		fail(w, 409, "A doctor with this slug already exists")
		return
	}
	id, _ := res.LastInsertId()
	x, e := scanDoctor(a.db.QueryRow("SELECT "+docCols+" FROM doctors WHERE id=?", id))
	if e != nil {
		fail(w, 500, "Could not load doctor")
		return
	}
	jsonOut(w, 201, x)
}
func pathID(r *http.Request) (int64, error) { return strconv.ParseInt(r.PathValue("id"), 10, 64) }
func (a *app) updateDoctor(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid doctor id")
		return
	}
	var d doctor
	if decode(r, &d) != nil {
		fail(w, 400, "Invalid doctor data")
		return
	}
	if e := validateDoctor(&d); e != nil {
		fail(w, 422, e.Error())
		return
	}
	res, e := a.db.Exec(`UPDATE doctors SET name=?,slug=?,photo=?,specialty=?,qualifications=?,experience=?,bio=?,availability=?,phone=?,email=?,organization=?,featured=?,sort_order=?,published=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, d.Name, d.Slug, d.Photo, d.Specialty, d.Qualifications, d.Experience, d.Bio, d.Availability, d.Phone, d.Email, d.Organization, d.Featured, d.SortOrder, d.Published, id)
	if e != nil {
		fail(w, 409, "A doctor with this slug already exists")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Doctor not found")
		return
	}
	x, _ := scanDoctor(a.db.QueryRow("SELECT "+docCols+" FROM doctors WHERE id=?", id))
	jsonOut(w, 200, x)
}
func (a *app) deleteDoctor(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid doctor id")
		return
	}
	res, e := a.db.Exec("DELETE FROM doctors WHERE id=?", id)
	if e != nil {
		fail(w, 500, "Could not delete doctor")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Doctor not found")
		return
	}
	w.WriteHeader(204)
}

type content struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Group     string `json:"group"`
	Value     string `json:"value"`
	Type      string `json:"type"`
	UpdatedAt string `json:"updated_at"`
}

func (a *app) contentRows() (map[string]content, error) {
	rows, e := a.db.Query("SELECT key,label,group_name,value,type,updated_at FROM content ORDER BY group_name,key")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]content{}
	for rows.Next() {
		var c content
		if e := rows.Scan(&c.Key, &c.Label, &c.Group, &c.Value, &c.Type, &c.UpdatedAt); e != nil {
			return nil, e
		}
		out[c.Key] = c
	}
	return out, rows.Err()
}
func (a *app) listContent(w http.ResponseWriter, r *http.Request) {
	c, e := a.contentRows()
	if e != nil {
		fail(w, 500, "Could not load content")
		return
	}
	jsonOut(w, 200, c)
}
func (a *app) createContent(w http.ResponseWriter, r *http.Request) {
	var c content
	if decode(r, &c) != nil {
		fail(w, 400, "Invalid content field")
		return
	}
	c.Key = strings.TrimSpace(c.Key)
	c.Label = strings.TrimSpace(c.Label)
	c.Group = strings.TrimSpace(c.Group)
	if c.Key == "" || c.Label == "" || c.Group == "" {
		fail(w, 422, "Key, label and section are required")
		return
	}
	if c.Type == "" {
		c.Type = "text"
	}
	allowed := map[string]bool{"text": true, "textarea": true, "image": true, "json": true}
	if !allowed[c.Type] {
		fail(w, 422, "Unsupported field type")
		return
	}
	if c.Type == "json" && !json.Valid([]byte(c.Value)) {
		fail(w, 422, "JSON fields must contain valid JSON")
		return
	}
	_, err := a.db.Exec("INSERT INTO content(key,label,group_name,value,type) VALUES(?,?,?,?,?)", c.Key, c.Label, c.Group, c.Value, c.Type)
	if err != nil {
		fail(w, 409, "A content field with this key already exists")
		return
	}
	a.db.QueryRow("SELECT updated_at FROM content WHERE key=?", c.Key).Scan(&c.UpdatedAt)
	jsonOut(w, 201, c)
}
func (a *app) updateContent(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var x struct {
		Value string `json:"value"`
	}
	if decode(r, &x) != nil {
		fail(w, 400, "Invalid content")
		return
	}
	if len(x.Value) > 100000 {
		fail(w, 422, "Content is too long")
		return
	}
	var typ string
	if a.db.QueryRow("SELECT type FROM content WHERE key=?", key).Scan(&typ) != nil {
		fail(w, 404, "Content field not found")
		return
	}
	if typ == "json" && !json.Valid([]byte(x.Value)) {
		fail(w, 422, "This field must contain valid JSON")
		return
	}
	a.db.Exec("UPDATE content SET value=?,updated_at=CURRENT_TIMESTAMP WHERE key=?", x.Value, key)
	var c content
	a.db.QueryRow("SELECT key,label,group_name,value,type,updated_at FROM content WHERE key=?", key).Scan(&c.Key, &c.Label, &c.Group, &c.Value, &c.Type, &c.UpdatedAt)
	jsonOut(w, 200, c)
}
func (a *app) deleteContent(w http.ResponseWriter, r *http.Request) {
	res, err := a.db.Exec("DELETE FROM content WHERE key=?", r.PathValue("key"))
	if err != nil {
		fail(w, 500, "Could not delete content field")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Content field not found")
		return
	}
	w.WriteHeader(204)
}
func (a *app) publicSite(w http.ResponseWriter, r *http.Request) {
	c, e := a.contentRows()
	if e != nil {
		fail(w, 500, "Could not load site")
		return
	}
	flat := map[string]any{}
	for k, v := range c {
		if v.Type == "json" {
			var j any
			if json.Unmarshal([]byte(v.Value), &j) == nil {
				flat[k] = j
				continue
			}
		}
		flat[k] = v.Value
	}
	d, _ := a.doctors("WHERE published=1 ORDER BY sort_order,id")
	jsonOut(w, 200, map[string]any{"content": flat, "doctors": d, "updated_at": time.Now().UTC()})
}

type cmsPage struct {
	ID             int64        `json:"id"`
	Slug           string       `json:"slug"`
	Title          string       `json:"title"`
	NavLabel       string       `json:"nav_label"`
	Description    string       `json:"description"`
	SEOTitle       string       `json:"seo_title"`
	SEODescription string       `json:"seo_description"`
	Status         string       `json:"status"`
	SortOrder      int          `json:"sort_order"`
	UpdatedAt      string       `json:"updated_at"`
	Sections       []cmsSection `json:"sections"`
}
type cmsSection struct {
	ID        int64     `json:"id"`
	PageID    int64     `json:"page_id"`
	Key       string    `json:"section_key"`
	Kind      string    `json:"kind"`
	Label     string    `json:"label"`
	Eyebrow   string    `json:"eyebrow"`
	Heading   string    `json:"heading"`
	Body      string    `json:"body"`
	MediaURL  string    `json:"media_url"`
	CTALabel  string    `json:"cta_label"`
	CTAURL    string    `json:"cta_url"`
	DataJSON  string    `json:"data_json"`
	Visible   bool      `json:"visible"`
	SortOrder int       `json:"sort_order"`
	Items     []cmsItem `json:"items"`
}
type cmsItem struct {
	ID        int64  `json:"id"`
	SectionID int64  `json:"section_id"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle"`
	Body      string `json:"body"`
	ImageURL  string `json:"image_url"`
	LinkLabel string `json:"link_label"`
	LinkURL   string `json:"link_url"`
	MetaJSON  string `json:"meta_json"`
	SortOrder int    `json:"sort_order"`
	Published bool   `json:"published"`
}

func (a *app) cms(includeDrafts bool) ([]cmsPage, error) {
	q := "SELECT id,slug,title,nav_label,description,seo_title,seo_description,status,sort_order,updated_at FROM pages"
	if !includeDrafts {
		q += " WHERE status='published'"
	}
	q += " ORDER BY sort_order,id"
	rows, e := a.db.Query(q)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	pages := []cmsPage{}
	for rows.Next() {
		var p cmsPage
		if e := rows.Scan(&p.ID, &p.Slug, &p.Title, &p.NavLabel, &p.Description, &p.SEOTitle, &p.SEODescription, &p.Status, &p.SortOrder, &p.UpdatedAt); e != nil {
			return nil, e
		}
		sr, e := a.db.Query(`SELECT id,page_id,section_key,kind,label,eyebrow,heading,body,media_url,cta_label,cta_url,data_json,visible,sort_order FROM sections WHERE page_id=? ORDER BY sort_order,id`, p.ID)
		if e != nil {
			return nil, e
		}
		for sr.Next() {
			var s cmsSection
			var visible int
			if e := sr.Scan(&s.ID, &s.PageID, &s.Key, &s.Kind, &s.Label, &s.Eyebrow, &s.Heading, &s.Body, &s.MediaURL, &s.CTALabel, &s.CTAURL, &s.DataJSON, &visible, &s.SortOrder); e != nil {
				sr.Close()
				return nil, e
			}
			s.Visible = visible == 1
			if !includeDrafts && !s.Visible {
				continue
			}
			ir, e := a.db.Query(`SELECT id,section_id,title,subtitle,body,image_url,link_label,link_url,meta_json,sort_order,published FROM section_items WHERE section_id=? ORDER BY sort_order,id`, s.ID)
			if e != nil {
				sr.Close()
				return nil, e
			}
			for ir.Next() {
				var it cmsItem
				var pub int
				if e := ir.Scan(&it.ID, &it.SectionID, &it.Title, &it.Subtitle, &it.Body, &it.ImageURL, &it.LinkLabel, &it.LinkURL, &it.MetaJSON, &it.SortOrder, &pub); e != nil {
					ir.Close()
					sr.Close()
					return nil, e
				}
				it.Published = pub == 1
				if includeDrafts || it.Published {
					s.Items = append(s.Items, it)
				}
			}
			ir.Close()
			p.Sections = append(p.Sections, s)
		}
		sr.Close()
		pages = append(pages, p)
	}
	return pages, rows.Err()
}
func (a *app) publicCMS(w http.ResponseWriter, r *http.Request) {
	pages, e := a.cms(false)
	if e != nil {
		fail(w, 500, "Could not load website content")
		return
	}
	jsonOut(w, 200, map[string]any{"pages": pages})
}
func (a *app) listPages(w http.ResponseWriter, r *http.Request) {
	pages, e := a.cms(true)
	if e != nil {
		fail(w, 500, "Could not load pages")
		return
	}
	jsonOut(w, 200, pages)
}
func validSlug(s string) string {
	return strings.Trim(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-")), "-")
}
func (a *app) createPage(w http.ResponseWriter, r *http.Request) {
	var p cmsPage
	if decode(r, &p) != nil {
		fail(w, 400, "Invalid page data")
		return
	}
	p.Slug = validSlug(p.Slug)
	if p.Slug == "" || strings.TrimSpace(p.Title) == "" {
		fail(w, 422, "Page title and slug are required")
		return
	}
	if p.Status == "" {
		p.Status = "draft"
	}
	res, e := a.db.Exec(`INSERT INTO pages(slug,title,nav_label,description,seo_title,seo_description,status,sort_order) VALUES(?,?,?,?,?,?,?,?)`, p.Slug, p.Title, p.NavLabel, p.Description, p.SEOTitle, p.SEODescription, p.Status, p.SortOrder)
	if e != nil {
		fail(w, 409, "A page with this slug already exists")
		return
	}
	p.ID, _ = res.LastInsertId()
	p.Sections = []cmsSection{}
	jsonOut(w, 201, p)
}
func (a *app) updatePage(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid page id")
		return
	}
	var p cmsPage
	if decode(r, &p) != nil {
		fail(w, 400, "Invalid page data")
		return
	}
	p.Slug = validSlug(p.Slug)
	if p.Slug == "" || strings.TrimSpace(p.Title) == "" {
		fail(w, 422, "Page title and slug are required")
		return
	}
	res, e := a.db.Exec(`UPDATE pages SET slug=?,title=?,nav_label=?,description=?,seo_title=?,seo_description=?,status=?,sort_order=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, p.Slug, p.Title, p.NavLabel, p.Description, p.SEOTitle, p.SEODescription, p.Status, p.SortOrder, id)
	if e != nil {
		fail(w, 409, "A page with this slug already exists")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Page not found")
		return
	}
	p.ID = id
	jsonOut(w, 200, p)
}
func (a *app) deletePage(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid page id")
		return
	}
	res, e := a.db.Exec("DELETE FROM pages WHERE id=?", id)
	if e != nil {
		fail(w, 500, "Could not delete page")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Page not found")
		return
	}
	w.WriteHeader(204)
}
func (a *app) createSection(w http.ResponseWriter, r *http.Request) {
	pid, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid page id")
		return
	}
	var s cmsSection
	if decode(r, &s) != nil {
		fail(w, 400, "Invalid section data")
		return
	}
	s.Key = validSlug(s.Key)
	if s.Key == "" || strings.TrimSpace(s.Label) == "" {
		fail(w, 422, "Section label and key are required")
		return
	}
	if s.Kind == "" {
		s.Kind = "content"
	}
	if s.DataJSON == "" {
		s.DataJSON = "{}"
	}
	res, e := a.db.Exec(`INSERT INTO sections(page_id,section_key,kind,label,eyebrow,heading,body,media_url,cta_label,cta_url,data_json,visible,sort_order) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, pid, s.Key, s.Kind, s.Label, s.Eyebrow, s.Heading, s.Body, s.MediaURL, s.CTALabel, s.CTAURL, s.DataJSON, s.Visible, s.SortOrder)
	if e != nil {
		fail(w, 409, "This page already has a section with that key")
		return
	}
	s.ID, _ = res.LastInsertId()
	s.PageID = pid
	s.Items = []cmsItem{}
	jsonOut(w, 201, s)
}
func (a *app) updateSection(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid section id")
		return
	}
	var s cmsSection
	if decode(r, &s) != nil {
		fail(w, 400, "Invalid section data")
		return
	}
	s.Key = validSlug(s.Key)
	if s.DataJSON == "" {
		s.DataJSON = "{}"
	}
	res, e := a.db.Exec(`UPDATE sections SET section_key=?,kind=?,label=?,eyebrow=?,heading=?,body=?,media_url=?,cta_label=?,cta_url=?,data_json=?,visible=?,sort_order=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, s.Key, s.Kind, s.Label, s.Eyebrow, s.Heading, s.Body, s.MediaURL, s.CTALabel, s.CTAURL, s.DataJSON, s.Visible, s.SortOrder, id)
	if e != nil {
		fail(w, 409, "Section key must be unique on this page")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Section not found")
		return
	}
	s.ID = id
	jsonOut(w, 200, s)
}
func (a *app) deleteSection(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid section id")
		return
	}
	res, e := a.db.Exec("DELETE FROM sections WHERE id=?", id)
	if e != nil {
		fail(w, 500, "Could not delete section")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Section not found")
		return
	}
	w.WriteHeader(204)
}
func (a *app) createItem(w http.ResponseWriter, r *http.Request) {
	sid, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid section id")
		return
	}
	var it cmsItem
	if decode(r, &it) != nil {
		fail(w, 400, "Invalid item data")
		return
	}
	if it.MetaJSON == "" {
		it.MetaJSON = "{}"
	}
	res, e := a.db.Exec(`INSERT INTO section_items(section_id,title,subtitle,body,image_url,link_label,link_url,meta_json,sort_order,published) VALUES(?,?,?,?,?,?,?,?,?,?)`, sid, it.Title, it.Subtitle, it.Body, it.ImageURL, it.LinkLabel, it.LinkURL, it.MetaJSON, it.SortOrder, it.Published)
	if e != nil {
		fail(w, 500, "Could not add item")
		return
	}
	it.ID, _ = res.LastInsertId()
	it.SectionID = sid
	jsonOut(w, 201, it)
}
func (a *app) updateItem(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid item id")
		return
	}
	var it cmsItem
	if decode(r, &it) != nil {
		fail(w, 400, "Invalid item data")
		return
	}
	if it.MetaJSON == "" {
		it.MetaJSON = "{}"
	}
	res, e := a.db.Exec(`UPDATE section_items SET title=?,subtitle=?,body=?,image_url=?,link_label=?,link_url=?,meta_json=?,sort_order=?,published=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, it.Title, it.Subtitle, it.Body, it.ImageURL, it.LinkLabel, it.LinkURL, it.MetaJSON, it.SortOrder, it.Published, id)
	if e != nil {
		fail(w, 500, "Could not update item")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Item not found")
		return
	}
	it.ID = id
	jsonOut(w, 200, it)
}
func (a *app) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid item id")
		return
	}
	res, e := a.db.Exec("DELETE FROM section_items WHERE id=?", id)
	if e != nil {
		fail(w, 500, "Could not delete item")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Item not found")
		return
	}
	w.WriteHeader(204)
}
func saveUpload(h *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(h.Filename))
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".svg": true}
	if !allowed[ext] {
		return "", errors.New("Use JPG, PNG, WebP or SVG")
	}
	if h.Size > 5<<20 {
		return "", errors.New("Image must be smaller than 5 MB")
	}
	f, e := h.Open()
	if e != nil {
		return "", e
	}
	defer f.Close()
	name := fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), token()[:8], ext)
	dst, e := os.OpenFile(filepath.Join("data/uploads", name), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if e != nil {
		return "", e
	}
	defer dst.Close()
	_, e = io.Copy(dst, io.LimitReader(f, 5<<20+1))
	return "/uploads/" + name, e
}
func (a *app) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	if e := r.ParseMultipartForm(6 << 20); e != nil {
		fail(w, 413, "Image must be smaller than 5 MB")
		return
	}
	_, h, e := r.FormFile("image")
	if e != nil {
		fail(w, 400, "Choose an image")
		return
	}
	url, e := saveUpload(h)
	if e != nil {
		fail(w, 422, e.Error())
		return
	}
	res, _ := a.db.Exec("INSERT OR IGNORE INTO media_assets(name,url,mime_type,size_bytes,deletable) VALUES(?,?,?,?,1)", h.Filename, url, h.Header.Get("Content-Type"), h.Size)
	id, _ := res.LastInsertId()
	jsonOut(w, 201, map[string]any{"id": id, "name": h.Filename, "url": url, "alt_text": "", "mime_type": h.Header.Get("Content-Type"), "size_bytes": h.Size, "deletable": true})
}

type mediaAsset struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	AltText   string `json:"alt_text"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	Deletable bool   `json:"deletable"`
	CreatedAt string `json:"created_at"`
}

func (a *app) listMedia(w http.ResponseWriter, r *http.Request) {
	rows, e := a.db.Query("SELECT id,name,url,alt_text,mime_type,size_bytes,deletable,created_at FROM media_assets ORDER BY created_at DESC,id DESC")
	if e != nil {
		fail(w, 500, "Could not load media")
		return
	}
	defer rows.Close()
	out := []mediaAsset{}
	for rows.Next() {
		var m mediaAsset
		var d int
		if e := rows.Scan(&m.ID, &m.Name, &m.URL, &m.AltText, &m.MimeType, &m.SizeBytes, &d, &m.CreatedAt); e != nil {
			fail(w, 500, "Could not load media")
			return
		}
		m.Deletable = d == 1
		out = append(out, m)
	}
	jsonOut(w, 200, out)
}
func (a *app) updateMedia(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid media id")
		return
	}
	var x struct {
		AltText string `json:"alt_text"`
	}
	if decode(r, &x) != nil {
		fail(w, 400, "Invalid media data")
		return
	}
	res, e := a.db.Exec("UPDATE media_assets SET alt_text=? WHERE id=?", strings.TrimSpace(x.AltText), id)
	if e != nil {
		fail(w, 500, "Could not update media")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "Media not found")
		return
	}
	jsonOut(w, 200, map[string]any{"id": id, "alt_text": x.AltText})
}
func (a *app) deleteMedia(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		fail(w, 400, "Invalid media id")
		return
	}
	var url string
	var deletable int
	if a.db.QueryRow("SELECT url,deletable FROM media_assets WHERE id=?", id).Scan(&url, &deletable) != nil {
		fail(w, 404, "Media not found")
		return
	}
	if deletable != 1 || !strings.HasPrefix(url, "/uploads/") {
		fail(w, 403, "Built-in website assets cannot be deleted")
		return
	}
	name := strings.TrimPrefix(url, "/uploads/")
	target := filepath.Join("data", "uploads", filepath.Base(name))
	if e := os.Remove(target); e != nil && !os.IsNotExist(e) {
		fail(w, 500, "Could not delete media file")
		return
	}
	a.db.Exec("DELETE FROM media_assets WHERE id=?", id)
	w.WriteHeader(204)
}
