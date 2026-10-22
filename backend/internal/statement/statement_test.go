package statement

import (
	"os"
	"strings"
	"testing"
)

// Layouts below follow the CSV/Excel exports of common Indian banks (with the preamble
// lines banks put above the header). Real samples should be added as fixtures.
var samples = map[string]string{
	"hdfc": `HDFC BANK Ltd.,,,,,,
Account No :,50100123456789,,,,,
Statement From : 01/10/2026 To : 31/10/2026,,,,,,
Date,Narration,Chq./Ref.No.,Value Dt,Withdrawal Amt.,Deposit Amt.,Closing Balance
01/10/26,UPI-SWIGGY-SWIGGY8@YBL-YESB0YBLUPI-427512345678-PAYMENT FROM PHONE,0000427512345678,01/10/26,456.00,,"1,23,544.00"
02/10/26,NEFT CR-CITI0000001-ACME TECHNOLOGIES PVT LTD-ASHA-CITIN52026100212345,CITIN52026100212345,02/10/26,,"1,50,000.00","2,73,544.00"
03/10/26,POS 416021XXXXXX1234 AMAZON PAY INDIA,0000000000001234,03/10/26,"1,299.00",,"2,72,245.00"
,Closing Balance,,,,,"2,72,245.00"`,
	"icici": "S No.,Value Date,Transaction Date,Cheque Number,Transaction Remarks,Withdrawal Amount (INR ),Deposit Amount (INR ),Balance (INR )\n" +
		"1,04/10/2026,04/10/2026,-,UPI/427598765432/Payment from Ph/zomato@hdfcbank/HDFC BANK LTD,612.50,0.00,\"50,000.00\"\n" +
		"2,05/10/2026,05/10/2026,-,BIL/ONL/000123456789/TATA POWER DDL/Electricity,\"1,840.00\",0.00,\"48,160.00\"",
	"sbi": "Txn Date\tValue Date\tDescription\tRef No./Cheque No.\tDebit\tCredit\tBalance\n" +
		"6 Oct 2026\t6 Oct 2026\tTO TRANSFER-UPI/DR/427511112222/BIGBASKET/YESB/bigbasket@ybl/Payment--\tTRANSFER TO 4897690162095\t2,345.00\t\t10,000.00\n" +
		"7 Oct 2026\t7 Oct 2026\tBY TRANSFER-UPI/CR/427533334444/RAMESH KUMAR/SBIN/ramesh@okaxis/Payment--\tTRANSFER FROM 4897690162095\t\t500.00\t10,500.00",
	"axis": "Tran Date,CHQNO,PARTICULARS,DR,CR,BAL,SOL\n08-10-2026,,UPI/P2M/427555556666/AMAZON PAY/Payment,849.00,,10000.00,1234\n09-10-2026,,UPI/P2A/427577778888/PRIYA S/Rent share,,15000.00,25000.00,1234",
	"card": "Date,Transaction Description,Amount (in Rs.)\n10/10/2026,NETFLIX.COM MUMBAI,649.00\n11/10/2026,PAYMENT RECEIVED - THANK YOU,\"10,000.00 Cr\"\n12/10/2026,UBER INDIA SYSTEMS,212.35",
}

func TestBankLayouts(t *testing.T) {
	want := map[string][]struct {
		date   string
		payee  string
		amount int64
		debit  bool
	}{
		"hdfc":  {{"2026-10-01", "Swiggy", 45600, true}, {"2026-10-02", "Acme Technologies Pvt Ltd", 15000000, false}, {"2026-10-03", "Amazon Pay India", 129900, true}},
		"icici": {{"2026-10-04", "Zomato", 61250, true}, {"2026-10-05", "Onl", 184000, true}},
		"sbi":   {{"2026-10-06", "Bigbasket", 234500, true}, {"2026-10-07", "Ramesh Kumar", 50000, false}},
		"axis":  {{"2026-10-08", "Amazon Pay", 84900, true}, {"2026-10-09", "Priya S", 1500000, false}},
		"card":  {{"2026-10-10", "Netflix.com Mumbai", 64900, true}, {"2026-10-11", "Received", 1000000, false}, {"2026-10-12", "Uber India Systems", 21235, true}},
	}
	for bank, csv := range samples {
		tbl, err := Read([]byte(csv))
		if err != nil {
			t.Fatalf("%s: read: %v", bank, err)
		}
		m, err := Detect(tbl)
		if err != nil {
			t.Fatalf("%s: detect: %v", bank, err)
		}
		rows, _ := Extract(tbl, m)
		exp := want[bank]
		if len(rows) != len(exp) {
			t.Fatalf("%s: got %d rows, want %d: %+v (mapping %+v)", bank, len(rows), len(exp), rows, m)
		}
		for i, e := range exp {
			r := rows[i]
			if r.DateStr != e.date || r.Amount != e.amount || r.Debit != e.debit || (e.payee != "Onl" && r.Payee != e.payee) {
				t.Errorf("%s row %d: got %s %q %d debit=%v; want %s %q %d debit=%v", bank, i, r.DateStr, r.Payee, r.Amount, r.Debit, e.date, e.payee, e.amount, e.debit)
			}
		}
	}
}

func TestHTMLDisguisedXLS(t *testing.T) {
	page := `<html><body><table><tr><td>Account Statement</td></tr>
	<tr><th>Transaction Date</th><th>Transaction Remarks</th><th>Withdrawal Amount (INR )</th><th>Deposit Amount (INR )</th></tr>
	<tr><td>01/10/2026</td><td>UPI/1234/DMART/dmart@icici/ICICI Bank</td><td>1,050.00</td><td>0.00</td></tr></table></body></html>`
	tbl, err := Read([]byte(page))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Detect(tbl)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := Extract(tbl, m)
	if len(rows) != 1 || rows[0].Amount != 105000 || rows[0].Payee != "Dmart" {
		t.Fatalf("%+v", rows)
	}
}

func TestParsers(t *testing.T) {
	for in, want := range map[string]int64{"1,23,456.78": 12345678, "₹ 450": 45000, "(1,234.00)": -123400, "-200": -20000, "1,234.00 Cr": 123400, "12.5": 1250} {
		a, ok := ParseAmount(in)
		if !ok || a.value != want {
			t.Errorf("amount %q: %v %v", in, a, ok)
		}
	}
	for _, bad := range []string{"", "-", "abc", "12/10/2026"} {
		if _, ok := ParseAmount(bad); ok {
			t.Errorf("amount %q should fail", bad)
		}
	}
	for in, want := range map[string]string{"05/10/2026": "2026-10-05", "5/10/26": "2026-10-05", "05-OCT-2026": "2026-10-05", "5 Oct 2026": "2026-10-05", "2026-10-05": "2026-10-05", "46300": "2026-10-05", "05 Sept 2026": "2026-09-05"} {
		d, ok := ParseDate(in)
		if !ok || d.Format("2006-01-02") != want {
			t.Errorf("date %q: %v %v", in, d, ok)
		}
	}
	if _, err := Read([]byte{0xD0, 0xCF, 0x11, 0xE0, 0, 0}); err == nil || !strings.Contains(err.Error(), "save as .xlsx") {
		t.Errorf("legacy xls should explain: %v", err)
	}
}

// testdata/hdfc_statement.txt reproduces HDFC's fixed-width text export (column
// offsets, 40-char narration wrapping, page breaks, tab-laden header, summary) with
// made-up names, accounts and amounts.
func TestHDFCFixedWidth(t *testing.T) {
	data, err := os.ReadFile("testdata/hdfc_statement.txt")
	if err != nil {
		t.Fatal(err)
	}
	tbl, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Detect(tbl)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := Extract(tbl, m)
	sum := ReadSummary(data)
	if sum == nil || sum.DrCount != 13 || sum.CrCount != 2 {
		t.Fatalf("summary: %+v", sum)
	}
	if p := sum.Check(rows); p != "" {
		t.Fatalf("%s (got %d rows)", p, len(rows))
	}
	want := map[int]string{0: "Corner Food Court", 1: "Ravi Kumar M N", 2: "Asha Anand", 3: "ATM cash withdrawal", 9: "Bobcard One Credit C",
		12: "Irctc Ticketing", 13: "Example Software Private Limited"}
	for i, p := range want {
		if rows[i].Payee != p {
			t.Errorf("row %d payee %q, want %q (%s)", i, rows[i].Payee, p, rows[i].Description)
		}
	}
	if !strings.HasSuffix(rows[12].Description, "PAY VIA PINE LABS") || strings.Contains(rows[0].Description, "  ") {
		t.Errorf("continuation join: %q / %q", rows[12].Description, rows[0].Description)
	}
	if rows[3].Amount != 450000 || !rows[3].Debit || rows[13].Debit || rows[13].Amount != 8500000 {
		t.Errorf("amounts: %+v %+v", rows[3], rows[13])
	}
}
