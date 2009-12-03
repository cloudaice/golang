// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes";
	"flag";
	"fmt";
	"go/ast";
	"go/parser";
	"go/printer";
	"go/scanner";
	"io";
	"os";
	pathutil "path";
	"strings";
)


var (
	// main operation modes
	list		= flag.Bool("l", false, "list files whose formatting differs from gofmt's");
	write		= flag.Bool("w", false, "write result to (source) file instead of stdout");
	rewriteRule	= flag.String("r", "", "rewrite rule (e.g., 'α[β:len(α)] -> α[β:]')");

	// debugging support
	comments	= flag.Bool("comments", true, "print comments");
	trace		= flag.Bool("trace", false, "print parse trace");

	// layout control
	tabwidth	= flag.Int("tabwidth", 8, "tab width");
	tabindent	= flag.Bool("tabindent", false, "indent with tabs independent of -spaces");
	usespaces	= flag.Bool("spaces", false, "align with spaces instead of tabs");
)


var (
	exitCode	= 0;
	rewrite		func(*ast.File) *ast.File;
	parserMode	uint;
	printerMode	uint;
)


func report(err os.Error) {
	scanner.PrintError(os.Stderr, err);
	exitCode = 2;
}


func usage() {
	fmt.Fprintf(os.Stderr, "usage: gofmt [flags] [path ...]\n");
	flag.PrintDefaults();
	os.Exit(2);
}


func initParserMode() {
	parserMode = uint(0);
	if *comments {
		parserMode |= parser.ParseComments
	}
	if *trace {
		parserMode |= parser.Trace
	}
}


func initPrinterMode() {
	printerMode = uint(0);
	if *tabindent {
		printerMode |= printer.TabIndent
	}
	if *usespaces {
		printerMode |= printer.UseSpaces
	}
}


func isGoFile(d *os.Dir) bool {
	// ignore non-Go files
	return d.IsRegular() && !strings.HasPrefix(d.Name, ".") && strings.HasSuffix(d.Name, ".go")
}


func processFile(f *os.File) os.Error {
	src, err := io.ReadAll(f);
	if err != nil {
		return err
	}

	file, err := parser.ParseFile(f.Name(), src, parserMode);
	if err != nil {
		return err
	}

	if rewrite != nil {
		file = rewrite(file)
	}

	var res bytes.Buffer;
	_, err = (&printer.Config{printerMode, *tabwidth, nil}).Fprint(&res, file);
	if err != nil {
		return err
	}

	if bytes.Compare(src, res.Bytes()) != 0 {
		// formatting has changed
		if *list {
			fmt.Fprintln(os.Stdout, f.Name())
		}
		if *write {
			err = io.WriteFile(f.Name(), res.Bytes(), 0);
			if err != nil {
				return err
			}
		}
	}

	if !*list && !*write {
		_, err = os.Stdout.Write(res.Bytes())
	}

	return err;
}


func processFileByName(filename string) (err os.Error) {
	file, err := os.Open(filename, os.O_RDONLY, 0);
	if err != nil {
		return
	}
	defer file.Close();
	return processFile(file);
}


type fileVisitor chan os.Error

func (v fileVisitor) VisitDir(path string, d *os.Dir) bool {
	return true
}


func (v fileVisitor) VisitFile(path string, d *os.Dir) {
	if isGoFile(d) {
		v <- nil;	// synchronize error handler
		if err := processFileByName(path); err != nil {
			v <- err
		}
	}
}


func walkDir(path string) {
	// start an error handler
	done := make(chan bool);
	v := make(fileVisitor);
	go func() {
		for err := range v {
			if err != nil {
				report(err)
			}
		}
		done <- true;
	}();
	// walk the tree
	pathutil.Walk(path, v, v);
	close(v);	// terminate error handler loop
	<-done;		// wait for all errors to be reported
}


func main() {
	flag.Usage = usage;
	flag.Parse();
	if *tabwidth < 0 {
		fmt.Fprintf(os.Stderr, "negative tabwidth %d\n", *tabwidth);
		os.Exit(2);
	}

	initParserMode();
	initPrinterMode();
	initRewrite();

	if flag.NArg() == 0 {
		if err := processFile(os.Stdin); err != nil {
			report(err)
		}
	}

	for i := 0; i < flag.NArg(); i++ {
		path := flag.Arg(i);
		switch dir, err := os.Stat(path); {
		case err != nil:
			report(err)
		case dir.IsRegular():
			if err := processFileByName(path); err != nil {
				report(err)
			}
		case dir.IsDirectory():
			walkDir(path)
		}
	}

	os.Exit(exitCode);
}
