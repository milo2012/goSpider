package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"net"
	"strings"
	"regexp"
	"flag"
	"sync"
	"time"
	"log"
	"os"
	"strconv"
	"crypto/tls"
	"github.com/PuerkitoBio/goquery"
	"github.com/asaskevich/govalidator"
)

const Version = "1.0"
const BodyLimit = 1024*1024
const MaxQueuedUrls = 4096
const MaxVisitedUrls = 8192
const UserAgent = "dcrawl/1.0"


var tmpUrlList []string
var tmpUrlList1 []string
var tmpPathList []string
var tmpFinalList []string

var mu sync.Mutex
var wg1 sync.WaitGroup

var domainName=""
var scheme=""

var rurls []string

var http_client *http.Client
var orignalURL=""

var (
	start_url = flag.String("url", "", "URL to start scraping from")
	max_threads = flag.Int("n", 8, "number of concurrent threads (def. 8)")
	max_urls_per_domain = flag.Int("mu", 5, "maximum number of links to spider per hostname (def. 5)")
	verbose = flag.Bool("v", false, "verbose (def. false)")
	logFilename = flag.String("l", "", "Filename to log results to")
	maxRepeatPaths = flag.Int("p", 999, "maximum number of repeat URI paths before terminating (def. 10)")
)

type ParsedUrl struct {
	u string
	urls []string
}
func stringInArray(s string, sa []string) (bool) {
	for _, x := range sa {
		if x == s {
			return true
		}
	}
	return false
}
func checkStatusCode(newUrl string) (bool) {
	var timeoutSec=5
	var userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.27 Safari/537.36"
	timeout := time.Duration(time.Duration(timeoutSec) * time.Second)
	client := http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
        	return http.ErrUseLastResponse
    	},		
	}
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	req, err := http.NewRequest("GET", newUrl, nil)
	if err==nil {
		req.Header.Add("User-Agent", userAgent)
		resp, err := client.Do(req)		
		if err==nil{					
			if resp.StatusCode!=200 && resp.StatusCode!=401 && resp.StatusCode!=405{
				return false 
			} else {
				return true
			}
		} else {
			return false
		}
	} else {
		return false
	}
	return false
}

func get_html(u string) ([]byte, error) {
	req, err := http.NewRequest("HEAD", u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	resp, err := http_client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP response %d", resp.StatusCode)
	}
	if _, ct_ok := resp.Header["Content-Type"]; ct_ok {
		ctypes := strings.Split(resp.Header["Content-Type"][0], ";")
		if !stringInArray("text/html", ctypes) {
			return nil, fmt.Errorf("URL is not 'text/html'")
		}
	}

	req.Method = "GET"
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	resp, err = http_client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(io.LimitReader(resp.Body, BodyLimit)) // limit response reading to 1MB
	//fmt.Println(string(b))
	if err != nil {
		return nil, err
	}
	return b, nil
}

func stringInSlice(str string, list []string) bool {
 	for _, v := range list {
 		if v == str {
 			return true
 		}
 	}
 	return false
}

func getBody(u string) ([]byte) {	
	var timeoutSec=10
	var userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.27 Safari/537.36"
	var timeout = time.Duration(time.Duration(timeoutSec) * time.Second)
	var client = http.Client{
		Timeout: timeout,
	}

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	req, err := http.NewRequest("GET", u, nil)
	if err==nil {
		req.Header.Add("User-Agent", userAgent)
		resp, err := client.Do(req)		
		body, err := ioutil.ReadAll(resp.Body)
		_=resp
		_=err
		return body
	}
	return nil
}

func checkURL(v string) bool {
	tmpSplit :=strings.Split(v,"\n")
	v=tmpSplit[0]
	if strings.HasPrefix(v,"//") || strings.HasPrefix(v,"http")  {
		if !strings.Contains(v," ") {
			valid := govalidator.IsRequestURL(v)			
			u, err := url.Parse(v)
			if err==nil {
				if valid==true {
					if u.Host!="bugs.jqueryui.com" && u.Host!="www.youtube.com" && u.Host!="schemas.xmlsoap.org" && u.Host!="jqueryui.com" && u.Host!="www.w3.org" && u.Host!="schemas.microsoft.com" && u.Host!="bugs.jquery.com" {
						return true
					}
				}
			}
			_=u
			return false
		}
	}
	return false
}

func extractURL(urlChan chan string) {
	//currentURL string, u string
	//r, _ := regexp.Compile(`<a\s+(?:[^>]*?\s+)?href=["\']([^"\']*)`)
	for x := range urlChan {
		var tmpUrl = strings.Split(x," | ")
    	var currentURL = tmpUrl[0]
    	var u = tmpUrl[1]
    	
		if strings.HasPrefix(currentURL,"http") {
			if !strings.Contains(currentURL,"mailto:") && !strings.Contains(currentURL,"javascript:") && currentURL!="#" {
				var tmpUrl=currentURL
				u2, err := url.Parse(tmpUrl)
				if err==nil {
					if u2.Host==domainName {
						var tmpUrl1=tmpUrl
						if !stringInSlice(tmpUrl1,rurls) {
							if !stringInSlice(tmpUrl1,tmpFinalList) {
								if checkStatusCode(tmpUrl1)==true {
									fmt.Println("[link] "+tmpUrl1)
									log.Println(tmpUrl1)
									tmpFinalList=append(tmpFinalList,tmpUrl1)
								}
							}
							rurls = append(rurls, tmpUrl1)	
						}
						_=tmpUrl1
					}
				}
			}
		} else {
			if strings.HasPrefix(currentURL,"?") {
				u2, err := url.Parse(u)
				if currentURL!="?"+u2.RawQuery {
					var tmpUrl1=u+currentURL
					if !stringInSlice(tmpUrl1,rurls) {
						if !stringInSlice(tmpUrl1,tmpFinalList) {
							if checkStatusCode(tmpUrl1)==true {
								fmt.Println("[link] "+tmpUrl1)
								log.Println(tmpUrl1)
								tmpFinalList=append(tmpFinalList,tmpUrl1)
							}
						}
						if !stringInSlice(tmpUrl1,tmpFinalList) {
							tmpFinalList=append(tmpFinalList,tmpUrl1)
						}						
						rurls = append(rurls, tmpUrl1)
					}
				}
				_=err
			} else {
				if !strings.HasPrefix(currentURL,"http") {
					if !strings.Contains(currentURL,"mailto:") && !strings.Contains(currentURL,"javascript:") && currentURL!="#" {
						u2, err := url.Parse(u)				
						var tmpPath=currentURL
						var tmpUrl1=""
						if strings.HasPrefix(string(tmpPath),"../") {
							tmpPath=tmpPath[2:len(tmpPath)]
						}
						if !strings.HasPrefix(string(tmpPath),"//") {
							tmpUrl1="http:"+string(tmpPath)
						}
						if !strings.HasPrefix(string(tmpPath),"/") {
							tmpUrl1=u2.Scheme+"://"+u2.Host+"/"+tmpPath
						} else {
							tmpUrl1=u2.Scheme+"://"+u2.Host+tmpPath
						}
						if !stringInSlice(tmpUrl1,rurls) {	
							if !stringInSlice(tmpUrl1,tmpFinalList) {
								if checkStatusCode(tmpUrl1)==true {
									fmt.Println("[link] "+tmpUrl1)
									log.Println(tmpUrl1)
									tmpFinalList=append(tmpFinalList,tmpUrl1)
								}
							}							
							rurls = append(rurls, tmpUrl1)
						}
						_=tmpPath
						_=tmpUrl1
						_=err
					}
				}
			}
		}
	}
}

func find_all_urls(u string, b []byte) ([]string) {
	//var rurls []string

	doc, err := goquery.NewDocument(u)
	if err != nil {
	    log.Fatal(err)
	}
	doc.Find("form").Each(func(index int, item *goquery.Selection) {
		linkTag := item
		link, _ := linkTag.Attr("action")
		if len(link)>0 {
			if strings.HasPrefix(link,"/") {
				u2, err := url.Parse(u)				
				var tmpUrl1=u2.Scheme+"://"+u2.Host+link
				if !stringInSlice(tmpUrl1,rurls) {
					rurls = append(rurls, tmpUrl1)	
				}
				_=err
			} else {
				if strings.HasPrefix(link,"http") {
					var tmpUrl1=link
					if !stringInSlice(tmpUrl1,rurls) {
						rurls = append(rurls, tmpUrl1)	
					}					
				} else {
					u2, err := url.Parse(u)				
					var tmpUrl1=u2.Scheme+"://"+u2.Host+"/"+link
					if !stringInSlice(tmpUrl1,rurls) {
						rurls = append(rurls, tmpUrl1)	
					}
					_=err
				}
			}
		}
	})

	r1, _ := regexp.Compile(`<script.*?src="(.*?)"`)
	urls1 := r1.FindAllSubmatch(b,-1)
	for _, x := range urls1 {
		tmpSplit1 :=strings.Split(string(x[0]),"src=\"")
		var tmpStr1 = tmpSplit1[1]
		if strings.HasSuffix(tmpStr1,"\"") {
			tmpStr1=tmpStr1[0:len(tmpStr1)-1]
			if strings.HasPrefix(tmpStr1,"/") {
				if checkURL(orignalURL+tmpStr1)==true {
					var scriptURL=orignalURL+tmpStr1
					if !stringInSlice(scriptURL,tmpFinalList) {
						if checkStatusCode(scriptURL)==true {
							fmt.Println("[script] "+scriptURL)
							log.Println(scriptURL)
							tmpFinalList=append(tmpFinalList,scriptURL)
						}
					}
					if !stringInSlice(scriptURL,tmpFinalList) {
						tmpFinalList=append(tmpFinalList,scriptURL)
					}						
					var bodyBytes=getBody(scriptURL)

					r2, _ := regexp.Compile(`((?:[a-zA-Z]{1,10}://|//)[^"'/]{1,}\.[a-zA-Z]{2,}[^"']{0,})`)
					urls2 := r2.FindAllSubmatch(bodyBytes,-1)
					for _, x1 := range urls2 {
						var newUrl=string(x1[0])
						if checkURL(newUrl)==true {
							if strings.HasPrefix(newUrl,"//") {
								newUrl="http://"+newUrl
							}
							if checkURL(newUrl)==true {
								splitStr :=strings.Split(newUrl,"\n")
								newUrl=splitStr[0]
								if strings.Contains(newUrl," ") {
									if !stringInSlice(newUrl,tmpFinalList) {
										if checkStatusCode(newUrl)==true {
											fmt.Println("[javascript] "+newUrl)												
											log.Println("[javascript] "+newUrl)												
											tmpFinalList=append(tmpFinalList,newUrl)
										}
									}
								}
							}
						}
					}
					//if strings.Contains(tmpStr1,"static/app/services/itemsService.js") || 
					//if strings.Contains(tmpStr1,"/static/app/post.js") {
					//var r3, _ = regexp.Compile(`[a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/]{1,}\.[a-zA-Z]{1,4}`)
					var r3, _ = regexp.Compile(`((?:/|\.\./|\./)[^"'><,;| *()(%%$^/\\\[\]][^"'><,;|()]{1,})`)
					var urls3 = r3.FindAllSubmatch(bodyBytes,-1)
					for _, x1 := range urls3 {
						var newUrl=string(x1[0])
						if strings.Contains(newUrl,".com") && strings.HasPrefix(newUrl,"/") { 
							newUrl="http://"+newUrl
						} else {
							newUrl=orignalURL+newUrl
						}
						if !stringInSlice(newUrl,tmpFinalList) {
							if checkStatusCode(newUrl)==true {
								fmt.Println("[javascript] "+newUrl)
								log.Println(newUrl)
								tmpFinalList=append(tmpFinalList,newUrl)
							}
						}
						if !stringInSlice(newUrl,tmpFinalList) {
							tmpFinalList=append(tmpFinalList,newUrl)
						}

						if checkURL(newUrl)==true {
							if strings.HasPrefix(newUrl,"//") {
								newUrl="http://"+newUrl
							}
							if checkURL(newUrl)==true {
								splitStr :=strings.Split(newUrl,"\n")
								newUrl=splitStr[0]
								if !stringInSlice(newUrl,tmpFinalList) {
									if checkStatusCode(newUrl)==true {
										fmt.Println("[javascript] "+newUrl)								
										log.Println(newUrl)								
										tmpFinalList=append(tmpFinalList,newUrl)
									}
								}
							}
						}
					}	
				} 
			}
		}
	}

	r, _ := regexp.Compile(`<a\s+(?:[^>]*?\s+)?href=["\']([^"\']*)`)
	urls := r.FindAllSubmatch(b,-1)

	var workersCount=10
	var wg sync.WaitGroup
	var urlChan = make(chan string)
	wg.Add(workersCount)

	for i := 0; i < workersCount; i++ {
		go func() {
			extractURL(urlChan)
			wg.Done()
		}()
	}
	for _, ua := range urls {
		urlChan <- string(ua[1])+" | "+u
	}
	close(urlChan) 
	/*		
	for _, ua := range urls {		
		if strings.HasPrefix(string(ua[1]),"http") {
			if !strings.Contains(string(ua[1]),"mailto:") && !strings.Contains(string(ua[1]),"javascript:") && string(ua[1])!="#" {
				var tmpUrl=string(ua[1])
				u2, err := url.Parse(tmpUrl)
				if err==nil {
					if u2.Host==domainName {
						var tmpUrl1=tmpUrl
						//var tmpUrl1=u2.Scheme+"://"+u2.Host+u2.Path+string(ua[1])
						if !stringInSlice(tmpUrl1,rurls) {
							if !stringInSlice(tmpUrl1,tmpFinalList) {
								if checkStatusCode(tmpUrl1)==true {
									fmt.Println("[link] "+tmpUrl1)
									log.Println(tmpUrl1)
									tmpFinalList=append(tmpFinalList,tmpUrl1)
								}
							}
							//fmt.Println(string(ua[1]))
							rurls = append(rurls, tmpUrl1)	
						}
						_=tmpUrl1
					}
				}
			}
		} else {
			if strings.HasPrefix(string(ua[1]),"?") {
				u2, err := url.Parse(u)
				if string(ua[1])!="?"+u2.RawQuery {
					var tmpUrl1=u+string(ua[1])
					if !stringInSlice(tmpUrl1,rurls) {
						if !stringInSlice(tmpUrl1,tmpFinalList) {
							if checkStatusCode(tmpUrl1)==true {
								fmt.Println("[link] "+tmpUrl1)
								log.Println(tmpUrl1)
								tmpFinalList=append(tmpFinalList,tmpUrl1)
							}
						}
						if !stringInSlice(tmpUrl1,tmpFinalList) {
							tmpFinalList=append(tmpFinalList,tmpUrl1)
						}						
						rurls = append(rurls, tmpUrl1)
					}
				}
				_=err
			} else {
				if !strings.HasPrefix(string(ua[1]),"http") {
					if !strings.Contains(string(ua[1]),"mailto:") && !strings.Contains(string(ua[1]),"javascript:") && string(ua[1])!="#" {
						u2, err := url.Parse(u)				
						var tmpPath=string(ua[1])
						var tmpUrl1=""
						if strings.HasPrefix(string(tmpPath),"../") {
							tmpPath=tmpPath[2:len(tmpPath)]
							//fmt.Println("x "+tmpPath)
						}
						if !strings.HasPrefix(string(tmpPath),"//") {
							tmpUrl1="http:"+string(tmpPath)
						}
						if !strings.HasPrefix(string(tmpPath),"/") {
							tmpUrl1=u2.Scheme+"://"+u2.Host+"/"+tmpPath
						} else {
							tmpUrl1=u2.Scheme+"://"+u2.Host+tmpPath
						}
						if !stringInSlice(tmpUrl1,rurls) {	
							if !stringInSlice(tmpUrl1,tmpFinalList) {
								if checkStatusCode(tmpUrl1)==true {
									fmt.Println("[link] "+tmpUrl1)
									log.Println(tmpUrl1)
									tmpFinalList=append(tmpFinalList,tmpUrl1)
								}
							}							
							rurls = append(rurls, tmpUrl1)
						}
						_=tmpPath
						_=tmpUrl1
						_=err
					}
				}
			}
		}
	}
	*/

	for _, tmpUrl := range rurls {
		if strings.Contains(tmpUrl,domainName) {
			if !stringInSlice(tmpUrl,rurls) {
				if !stringInSlice(tmpUrl,tmpFinalList) {
					if checkStatusCode(tmpUrl)==true {
						fmt.Println("[link] "+tmpUrl)
						log.Println(tmpUrl)
						tmpFinalList=append(tmpFinalList,tmpUrl)
					}
				}				
				rurls = append(rurls, tmpUrl)
			}
		}
	}
	return rurls
}

func grab_site_urls(u string) ([]string, error) {
	var ret []string
	b, err := get_html(u)
	if err == nil {
		ret = find_all_urls(u, b)
	}
	return ret, err
}

func process_urls(in <-chan string, out chan<- ParsedUrl) {
	for {
		var u string = <-in
		if *verbose {
			fmt.Printf("[->] %s\n", u)
		}
		fmt.Println("Processing: "+u)
		urls, err := grab_site_urls(u)
		if err != nil {
			u = ""
		}
		for _, tmpUrl := range urls {
			urls1, err := grab_site_urls(tmpUrl)			
			if err != nil {
				out <- ParsedUrl{u, urls1}
			}
		}
		out <- ParsedUrl{u, urls}
	}
}
 func dup_count(list []string) map[string]int {
 	duplicate_frequency := make(map[string]int)
 	for _, item := range list {
 		_, exist := duplicate_frequency[item]
 		if exist {
 			duplicate_frequency[item] += 1 // increase counter by 1 if already in the map
 		} else {
 			duplicate_frequency[item] = 1 // else start counting from 1
 		}
 	}
 	return duplicate_frequency
 }

func create_http_client() *http.Client {
	var transport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 20 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 15 * time.Second,
		DisableKeepAlives: true,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Timeout: time.Second * 30,
		Transport: transport,
	}
	return client
}

func banner() {
}

func usage() {
	fmt.Printf("usage: dcrawl -url URL -out OUTPUT_FILE\n\n")
}

func init() {
	http_client = create_http_client()
}

func testFakePath(urlChan chan string) {
	//values := [][]string{}	
    for newUrl := range urlChan {
    	newUrl=strings.Replace(newUrl,"./","",1)
    	if !stringInSlice(newUrl,tmpFinalList) {
			if checkStatusCode(newUrl)==true {
	    		fmt.Println("[link] "+newUrl)
		    	log.Println(newUrl)
				tmpFinalList=append(tmpFinalList,newUrl)
			}
		}
		tmpUrlList=append(tmpUrlList,newUrl)
		tmpUrlList1=append(tmpUrlList1,newUrl)
		u, err := url.Parse(newUrl)
		tmpPathList = append(tmpPathList,u.Path)
		_=err

		urls, err := grab_site_urls(newUrl)			
		for _, u1 := range urls {
			u2, err := url.Parse(u1)
			if u2.Host==domainName {	
				if !stringInSlice(u1,tmpUrlList) {
					mu.Lock()	
					u1=strings.Replace(u1,"./","",1)	
					if !stringInSlice(u1,tmpFinalList) {
						if checkStatusCode(u1)==true {
							fmt.Println("[link] "+u1)	
							log.Println(u1)
							tmpFinalList=append(tmpFinalList,u1)
						}
					}	
					tmpUrlList=append(tmpUrlList,u1)
					tmpUrlList1=append(tmpUrlList1,u1)
					u, err := url.Parse(u1)
					tmpPathList = append(tmpPathList,u.Path)
					_=err
					mu.Unlock()
					//here				    
				    doc, err := goquery.NewDocument(u1)
				    if err != nil {
				        log.Fatal(err)
				    }
					doc.Find("a").Each(func(index int, item *goquery.Selection) {
						linkTag := item
						link, _ := linkTag.Attr("href")
						if link=="#" {
							link1, _ := item.Attr("onclick")
							tmpList1 := strings.Split(link1,"'")
							for _, x := range tmpList1 {
								if strings.Contains(x,"/") {
									if strings.HasPrefix(x,"./") {
										if checkStatusCode(orignalURL+x[1:len(x)])==true {
											fmt.Println("[link] "+orignalURL+x[1:len(x)])
											log.Println(orignalURL+x[1:len(x)])
											tmpUrlList=append(tmpUrlList,orignalURL+x[1:len(x)])
											tmpUrlList1=append(tmpUrlList1,orignalURL+x[1:len(x)])
											u, err := url.Parse(orignalURL+x[1:len(x)])
											tmpPathList = append(tmpPathList,u.Path)
											_=err
										}

									}
								}
							}
						} 
					})
					doc.Find("form").Each(func(index int, item *goquery.Selection) {
						linkTag := item
						link, _ := linkTag.Attr("action")
						if len(link)>0 {
							if strings.HasPrefix(link,"/") {
								mu.Lock()
								link=strings.Replace(link,"./","",1)													
								var tmpNewUrl=orignalURL+link	
								//Count number of duplicates in URI path
								dup_map := dup_count(tmpPathList)
								for k, v := range dup_map {
									if v>*maxRepeatPaths {
										fmt.Println("\n[*] Exceeded maximum number of repeated paths for: "+k+" ["+strconv.Itoa(*maxRepeatPaths)+"]")
										os.Exit(3)
									}
									_=k
 								}
											
								if !stringInSlice(tmpNewUrl,tmpUrlList) {										
									if checkURL(tmpNewUrl)==true {
										fmt.Println("[link] "+tmpNewUrl)
										log.Println(tmpNewUrl)
										tmpUrlList=append(tmpUrlList,tmpNewUrl)
										tmpUrlList1=append(tmpUrlList1,tmpNewUrl)
										u, err := url.Parse(tmpNewUrl)
										tmpPathList = append(tmpPathList,u.Path)
										_=err
									}
								}
								mu.Unlock()
							} else {
								if strings.HasPrefix(link,"http") {
									
									var tmpNewUrl=link	
									u, err := url.Parse(tmpNewUrl)
									if u.Host==domainName {
										mu.Lock()
										if !stringInSlice(tmpNewUrl,tmpUrlList) {
											if checkURL(tmpNewUrl)==true {
												fmt.Println("[link] "+tmpNewUrl)
												log.Println(tmpNewUrl)
												tmpUrlList=append(tmpUrlList,tmpNewUrl)
												tmpUrlList1=append(tmpUrlList1,tmpNewUrl)
												u, err := url.Parse(tmpNewUrl)
												tmpPathList = append(tmpPathList,u.Path)
												_=err
											}
										}
										mu.Unlock()
									}			
									_=err						
								} else {
									mu.Lock()
									link=strings.Replace(link,"./","",1)
									var tmpNewUrl=orignalURL+"/"+link	
									//u1, err := url.Parse(tmpNewUrl)		
									//fmt.Println("y "+u1.Path)		
									//fmt.Println("y "+link)
									_=err															
									if !stringInSlice(tmpNewUrl,tmpUrlList) {
										if checkURL(tmpNewUrl)==true {
											fmt.Println("[link] "+tmpNewUrl)
											log.Println(tmpNewUrl)
											tmpUrlList=append(tmpUrlList,tmpNewUrl)
											tmpUrlList1=append(tmpUrlList1,tmpNewUrl)
											u, err := url.Parse(tmpNewUrl)
											tmpPathList = append(tmpPathList,u.Path)
											_=err
										}
									}
									mu.Unlock()
								}
							}
						}
					})
				}
				_=err
			}
		}
		_=err
	}
	//for msg := range message {
	//	fmt.Println("c "+msg)
	//}
}	


func main() {
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))

	//banner()

	flag.Parse()
	//*max_threads=10
	*max_urls_per_domain=999

	fmt.Printf("\n")
	var tmpUrlList []string

	if len(*logFilename)>0 {
		logfileF, err := os.OpenFile(*logFilename, os.O_WRONLY|os.O_CREATE|os.O_APPEND,0644)
		if err != nil {
				log.Fatal(err)
		}   
		defer logfileF.Close()
		log.SetOutput(logfileF)
		fmt.Println("[*] Writing results to: "+*logFilename)
	} else {
		var tmpList1 = strings.Split(*start_url,"://")
		var tmpLogFilename="crawl_"+tmpList1[0]+"_"+tmpList1[1]+".log"
		if strings.HasSuffix(tmpLogFilename,"/.log") {
			tmpLogFilename = tmpLogFilename[0:len(tmpLogFilename)-5]
			tmpLogFilename = tmpLogFilename+".log"
		}

		fmt.Println("[*] Writing results to: "+tmpLogFilename)
		logfileF, err := os.OpenFile(tmpLogFilename, os.O_WRONLY|os.O_CREATE,0644)
		if err != nil {
				log.Fatal(err)
		}   
		defer logfileF.Close()
		log.SetOutput(logfileF)
	}

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	resp, err := http.Get(*start_url)
    if err != nil {
        log.Fatalf("http.Get => %v", err.Error())
    }
	finalURL := resp.Request.URL.String()
	u, err := url.Parse(finalURL)
	if err != nil {
		log.Fatal(err)
	}	

	domainName=u.Host
	scheme=u.Scheme
	orignalURL=u.Scheme+"://"+u.Host
	fmt.Println("Processing: "+finalURL)
	urls, err := grab_site_urls(*start_url)
	for _, u := range urls {
		u1, err := url.Parse(u)
		if u1.Host==domainName {
			tmpUrlList=append(tmpUrlList,u)
		}
		_=err
	}

	var workersCount=*max_threads
	urlChan := make(chan string)
	wg1.Add(workersCount)

	for i := 0; i < workersCount; i++ {
		go func() {
			defer wg1.Done()
			testFakePath(urlChan)
		}()
	}

	var processedUrlList []string
	var tmpComplete=false
	
	for tmpComplete==false {
		for i := 0; i < len(tmpUrlList); i++ {
			if !stringInSlice(tmpUrlList[i],processedUrlList) {
				urlChan <- tmpUrlList[i]
				processedUrlList=append(processedUrlList,tmpUrlList[i])
			}
		}		
		time.Sleep(5 * time.Second)
		var lastCount=0
		for _, each := range tmpUrlList1 {
			if !stringInSlice(each,tmpUrlList) {
				tmpUrlList=append(tmpUrlList,each)
				lastCount+=1
			}
		}
		if lastCount==0 {
			tmpComplete=true
		}	
	}
	close(urlChan)    
	wg1.Wait()
}