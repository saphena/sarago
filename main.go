package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/pkg/browser"

	_ "github.com/go-sql-driver/mysql"
	yaml "gopkg.in/yaml.v2"
)

var (
	port     = flag.String("port", "80", "the HTTP port")
	db3      = flag.String("db", "127.0.0.1", "IP of MySQL DB")
	starturl = flag.String("url", "http://localhost", "starting URL")
	pagesz   = flag.Int("pg", 15, "Pagesize for results")
	yml      = flag.String("cfg", "sarago.yml", "Path to config")
)

const myversion = "v3.0"

var Y struct {
	DatabaseHost string `yaml:"DatabaseHost"`
	DatabasePort string `yaml:"DatabasePort"`
	DatabaseName string `yaml:"DatabaseName"`
	DatabaseUser string `yaml:"DatabaseUser"`
	DatabasePass string `yaml:"DatabasePass"`
}
var sqldb *sql.DB

func formatCommas(num int) string {
	str := fmt.Sprintf("%d", num)
	re := regexp.MustCompile(`(\d+)(\d{3})`)
	for n := ""; n != str; {
		n = str
		str = re.ReplaceAllString(str, "$1,$2")
	}
	return str
}

func fetchTemplate(template string) string {

	file, err := os.Open(template)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err = file.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	b, err := io.ReadAll(file)
	return string(b)
}

func getValueFromDB(sql, colname, defvalue string) string {

	row, err := sqldb.Query(sql)
	if err != nil {
		log.Fatal(err)
	}
	defer row.Close()
	var res string = defvalue
	if row.Next() {
		var rex string
		row.Scan(&rex)
		res = rex
	}
	return res

}

func showDuration(duration string) string {

	var re = regexp.MustCompile(`\:`)
	var res string = ""
	hms := re.Split(duration, 3)
	h, _ := strconv.Atoi(hms[0])
	m, _ := strconv.Atoi(hms[1])
	s, _ := strconv.Atoi(hms[2])
	if h != 0 {
		res += strconv.Itoa(h) + "h "
		res += strconv.Itoa(m) + "m "
	} else if m != 0 {
		res += strconv.Itoa(m) + "m "
	}
	if s != 0 {
		res += strconv.Itoa(s) + "s "
	}
	return res
}

func showDate(dt string) string {

	layout := "2006-01-02"
	t, _ := time.Parse(layout, dt)

	return t.Format("2 Jan 2006")
}

func showDatetime(dt string) string {

	layout := "2006-01-02T15:04:05Z"
	t, _ := time.Parse(layout, dt)

	return t.Format("2 Jan 2006 3:04pm Mon")
}

func countrows(table, where string) int {

	var sql = "SELECT Count(1) As rex FROM " + table
	if where != "" {
		sql += " WHERE " + where
	}
	var res int
	res, _ = strconv.Atoi(getValueFromDB(sql, "rex", "-1"))
	return res

}

func lookupHandler(w http.ResponseWriter, r *http.Request) {

	if err := r.ParseForm(); err != nil {
		fmt.Fprintf(w, "ParseForm() err: %v", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	fmt.Fprint(w, fetchTemplate("htmlhead.html"))
	fmt.Fprintf(w, "<h2>%v</h2>", getValueFromDB("SELECT dbname FROM params", "dbname", ""))
	fmt.Fprint(w, fetchTemplate("htmllookup.html"))

	var tel string = r.FormValue("tel")
	var daterange bool = r.FormValue("dates") != "all"
	var fromdate string = r.FormValue(("fromdate"))
	var todate string = r.FormValue("todate")
	var offset int = 0

	offset, _ = strconv.Atoi(r.FormValue("offset"))

	fmt.Fprint(w, "<p>Showing ")
	if tel != "" {
		fmt.Fprintf(w, "%v", tel)
	} else {
		fmt.Fprint(w, "all calls")
	}
	if daterange {
		fmt.Fprint(w, "; Call date ")
		if fromdate == "" {
			fmt.Fprint(w, "upto "+showDate(todate))
		} else {
			fmt.Fprint(w, showDate(fromdate))
			if todate != "" {
				if todate != fromdate {
					fmt.Fprint(w, " - "+showDate(todate))
				}
			} else {
				fmt.Fprint(w, " onwards")
			}
		}
	}

	if todate != "" {
		todate += "T23:59:59"
	}

	var sql string = "SELECT cdrid,direction,duration,connected,ifnull(aphone,'') as aphone,ifnull(bphone,'') as bphone,folderid "
	var where string = ""
	sql += "FROM cdrs "
	sql += "WHERE "
	if tel != "" {
		where += "(aphone LIKE '%" + tel + "%' OR bphone LIKE '%" + tel + "%') "
	} else {
		where += "1=1 "
	}
	if daterange {
		if fromdate != "" {
			where += " AND connected >= '" + fromdate + "'"
		}
		if todate != "" {
			where += " AND connected <= '" + todate + "'"
		}
	}
	nresults := countrows("cdrs", where)

	fmt.Fprintf(w, "; %v found</p>", formatCommas(nresults))

	sql += where
	sql += " LIMIT " + strconv.Itoa(offset) + ", " + strconv.Itoa(*pagesz)

	fmt.Fprintf(w, "\n<!-- %v -->\n", sql)

	rows, err := sqldb.Query(sql)
	if err != nil {
		fmt.Fprintf(w, "OMG!!! %v", err)
		return
	}
	fmt.Fprint(w, "<table id=\"results\"><thead><tr>")
	fmt.Fprint(w, "<th class=\"duration\">Duration</th>")
	fmt.Fprint(w, "<th class=\"connected\">Connected</th>")
	fmt.Fprint(w, "<th class=\"direction\">I/O</th>")
	fmt.Fprint(w, "<th class=\"aphone\">From</th>")
	fmt.Fprint(w, "<th class=\"bphone\">To</th></tr></thead><tbody>")
	for rows.Next() {
		var cdrid string
		var direction string
		var duration string
		var connected string
		var aphone, bphone string
		var folderid int
		rows.Scan(&cdrid, &direction, &duration, &connected, &aphone, &bphone, &folderid)
		var record string = "/cdr" + strconv.Itoa(folderid) + "/{" + cdrid + "}.osf"
		fmt.Fprintf(w, "<tr><td class=\"duration\">%v</td>", showDuration(duration))
		fmt.Fprintf(w, "<td class=\"connected\">%v</td>", showDatetime(connected))
		fmt.Fprintf(w, "<td class=\"direction\">%v</td>", direction)
		fmt.Fprintf(w, "<td class=\"aphone\">%v</td><td class=\"bphone\">%v</td>", aphone, bphone)
		fmt.Fprint(w, "<td class=\"audio\"><audio controls><source src=")
		fmt.Fprintf(w, "\"%v\" type=\"audio/mpeg\"></audio></td>", record)
		fmt.Fprint(w, "</tr>")
	}
	fmt.Fprint(w, "</tbody></table>")

	if offset > 0 {
		fmt.Fprint(w, "<form action=\"lookup\" method=\"post\">")
		fmt.Fprint(w, "<input type=\"hidden\" name=\"tel\" value=\""+r.FormValue("tel")+"\">")
		fmt.Fprint(w, "<input type=\"hidden\" name=\"dates\" value=\""+r.FormValue("dates")+"\">")
		fmt.Fprint(w, "<input type=\"hidden\" name=\"fromdate\" value=\""+r.FormValue("fromdate")+"\">")
		fmt.Fprint(w, "<input type=\"hidden\" name=\"todate\" value=\""+r.FormValue("todate")+"\">")
		var poffset int = 0
		if offset >= *pagesz {
			poffset = offset - *pagesz
		}
		fmt.Fprint(w, "<input type=\"hidden\" name=\"offset\" value=\""+strconv.Itoa(poffset)+"\">")
		fmt.Fprint(w, "<input type=\"submit\" value=\"&NestedLessLess;\"> ")
		fmt.Fprint(w, "</form>")

	}

	if nresults-offset > *pagesz {
		fmt.Fprint(w, "<form action=\"lookup\" method=\"post\">")
		fmt.Fprint(w, "<input type=\"hidden\" name=\"tel\" value=\""+r.FormValue("tel")+"\">")
		fmt.Fprint(w, "<input type=\"hidden\" name=\"dates\" value=\""+r.FormValue("dates")+"\">")
		fmt.Fprint(w, "<input type=\"hidden\" name=\"fromdate\" value=\""+r.FormValue("fromdate")+"\">")
		fmt.Fprint(w, "<input type=\"hidden\" name=\"todate\" value=\""+r.FormValue("todate")+"\">")
		var poffset int = offset + *pagesz
		fmt.Fprint(w, "<input type=\"hidden\" name=\"offset\" value=\""+strconv.Itoa(poffset)+"\">")
		fmt.Fprint(w, "<input type=\"submit\" value=\"&NestedGreaterGreater;\"> ")
		fmt.Fprint(w, "</form>")

	}

}

func startServer() {

	numrex := countrows("cdrs", "")
	fmt.Printf("Number of CDRs: %v\n", formatCommas(numrex))
	browser.OpenURL(*starturl + ":" + *port)

}

func handleFolder(folderid int, datapath string) {

	cdrserver := http.FileServer(http.Dir(datapath))
	cdrf := "/cdr" + strconv.Itoa(folderid) + "/"
	http.Handle(cdrf, http.StripPrefix(cdrf, cdrserver))
	fmt.Printf("Voice Recordings folder [%v] - %v\n", folderid, datapath)

}

func handleFolders() {

	rows, err := sqldb.Query("SELECT folderid,datapath FROM folders")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var folderid int
		var datapath string
		rows.Scan(&folderid, &datapath)
		handleFolder(folderid, datapath)
	}

}

func configHandler(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	fmt.Fprint(w, fetchTemplate("htmlhead.html"))

	if err := r.ParseForm(); err == nil {

		var x, y string
		var updated bool = false

		x = r.FormValue("dbname")
		if x != "" {
			var sql string = "UPDATE params SET dbname=?"
			sqldb.Exec(sql, x)
			updated = true
		}
		x = r.FormValue("datapath")
		y = r.FormValue("folderid")
		if x != "" {
			var sql string = "UPDATE folders SET datapath=? WHERE folderid=?"
			sqldb.Exec(sql, x, y)
			updated = true
		}

		if updated {
			fmt.Fprintf(w, "<h2>%v</h2>", getValueFromDB("SELECT dbname FROM params", "dbname", ""))
			fmt.Fprint(w, fetchTemplate("htmllookup.html"))
			return
		}
	}

	var dbname string = getValueFromDB("SELECT dbname FROM params", "dbname", "*unknown*")

	fmt.Fprint(w, "<h2>Database configuration</h2>")

	fmt.Fprint(w, "<div id=\"dbconfig\">")
	fmt.Fprintf(w, "<p class=\"copyrite\">SARemote %v\nCopyright (c) Bob Stammers 2023", myversion)
	fmt.Fprint(w, "  (<a href=\"https://github.com/saphena/oldvr\" target=\"_blank\">https://github.com/saphena/oldvr</a>)</p>")
	host, _ := os.Hostname()
	dir, _ := os.Getwd()
	dir, _ = filepath.Abs(dir)
	fmt.Fprintf(w, "<p>Running on <strong>%v</strong> in <strong>%v</strong></p>", host, dir)
	fmt.Fprint(w, "<form action=\"config\" method=\"post\">")
	fmt.Fprint(w, "<label for=\"dbname\">DB description: </label>")
	fmt.Fprintf(w, "<input type=\"text\" id=\"dbname\" name=\"dbname\" value=\"%v\">", dbname)

	row, err := sqldb.Query("SELECT folderid,datapath FROM folders ORDER BY folderid")
	if err != nil {
		log.Fatal(err)
	}
	defer row.Close()

	fmt.Fprint(w, "<p>Folders containing voice recordings</p>")

	fmt.Fprint(w, "<ul id=\"folderlist\">")
	for row.Next() {
		var folderid int
		var datapath string
		row.Scan(&folderid, &datapath)
		fmt.Fprintf(w, "<li><input type=\"text\" name=\"folderid\" value=\"%v\" readonly> : ", folderid)
		fmt.Fprintf(w, "<input type=\"text\" name=\"datapath\" value=\"%v\"></li>", datapath)

	}
	fmt.Fprint(w, "</ul>")

	fmt.Fprint(w, "<input type=\"submit\" value=\"Update\">")
	fmt.Fprint(w, "</form>")
	fmt.Fprint(w, "</div>")

}

func main() {

	fmt.Printf("\nSARemote %v\nCopyright (c) Bob Stammers 2023\n", myversion)
	fmt.Printf("Architecture: %v\n\n", runtime.GOARCH)

	flag.Parse()

	var err error

	file, err := os.Open(*yml)
	if err == nil {

		defer file.Close()

		D := yaml.NewDecoder(file)
		D.Decode(&Y)
	}
	sqldb, err = sql.Open("mysql", Y.DatabaseUser+":"+Y.DatabasePass+"@"+Y.DatabaseHost+":"+Y.DatabasePort+"/"+Y.DatabaseName)
	if err != nil {
		log.Fatal(err)
	}
	defer sqldb.Close()

	fmt.Printf("CDRs: %v\n", *db3)
	fmt.Printf("Database: %v\n", getValueFromDB("SELECT dbname FROM params", "dbname", "unidentified"))
	fileServer := http.FileServer(http.Dir("."))
	http.Handle("/", fileServer)

	http.HandleFunc("/lookup", lookupHandler)
	http.HandleFunc("/config", configHandler)
	http.HandleFunc("/about", configHandler)

	handleFolders()

	go startServer()

	fmt.Print("Serving port " + *port + "\n")
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatal(err)
	}
}
