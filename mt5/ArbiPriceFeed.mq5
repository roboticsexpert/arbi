//+------------------------------------------------------------------+
//|                                              ArbiPriceFeed.mq5   |
//|  Pushes top-of-book metal quotes from MetaTrader 5 to the Arbi   |
//|  backend, which has no other way to reach them: MetaTrader has   |
//|  no server-side API, so the terminal has to dial out.            |
//|                                                                  |
//|  Setup:                                                          |
//|    1. Tools -> Options -> Expert Advisors                        |
//|       tick "Allow WebRequest for listed URL"                     |
//|       add the backend origin, e.g. http://127.0.0.1:8080         |
//|    2. Enable "Algo Trading" in the toolbar                       |
//|    3. Drop this EA on any chart and fill in Endpoint + Token     |
//+------------------------------------------------------------------+
#property copyright "Arbi"
#property version   "1.00"

input string InpEndpoint   = "http://127.0.0.1:8080/api/mt5/ticks"; // Backend ingest URL
input string InpToken      = "";                                    // Shared secret (MT5_INGEST_TOKEN)
input string InpSymbols    = "GOLD_kilogram,GOLD_ounce";            // Comma-separated MT5 symbols
input int    InpIntervalMs = 1000;                                  // Push interval (ms)
input int    InpTimeoutMs  = 5000;                                  // HTTP timeout (ms)
input bool   InpOnlyOnChange = true;                                // Skip push when nothing moved
input int    InpHeartbeatSec = 30;                                  // Push anyway every N sec (0 = never)

string   g_symbols[];
double   g_last_bid[];
double   g_last_ask[];
datetime g_last_push = 0;

//+------------------------------------------------------------------+
int OnInit()
  {
   if(StringLen(InpEndpoint) == 0)
     {
      Print("[Arbi] Endpoint is empty - nothing to push to.");
      return(INIT_PARAMETERS_INCORRECT);
     }
   if(StringLen(InpToken) == 0)
     {
      Print("[Arbi] Token is empty. The backend rejects untokenised pushes; set MT5_INGEST_TOKEN and paste it here.");
      return(INIT_PARAMETERS_INCORRECT);
     }

   int count = StringSplit(InpSymbols, ',', g_symbols);
   if(count <= 0)
     {
      Print("[Arbi] No symbols configured.");
      return(INIT_PARAMETERS_INCORRECT);
     }

   ArrayResize(g_last_bid, count);
   ArrayResize(g_last_ask, count);

   for(int i = 0; i < count; i++)
     {
      StringTrimLeft(g_symbols[i]);
      StringTrimRight(g_symbols[i]);
      g_last_bid[i] = 0.0;
      g_last_ask[i] = 0.0;

      // Symbols must sit in Market Watch before the terminal will quote them.
      if(!SymbolSelect(g_symbols[i], true))
         PrintFormat("[Arbi] Could not select symbol '%s' - check the exact name in Market Watch.", g_symbols[i]);
     }

   int interval = InpIntervalMs;
   if(interval < 100)
      interval = 100;
   EventSetMillisecondTimer(interval);

   PrintFormat("[Arbi] Feeding %d symbol(s) to %s every %d ms.", count, InpEndpoint, interval);
   return(INIT_SUCCEEDED);
  }

//+------------------------------------------------------------------+
void OnDeinit(const int reason)
  {
   EventKillTimer();
   Print("[Arbi] Price feed stopped.");
  }

//+------------------------------------------------------------------+
void OnTimer()
  {
   string   ticks = "";
   int      included = 0;
   bool     changed = false;
   datetime now = TimeCurrent();

   for(int i = 0; i < ArraySize(g_symbols); i++)
     {
      MqlTick tick;
      if(!SymbolInfoTick(g_symbols[i], tick))
         continue;
      if(tick.bid <= 0.0 || tick.ask <= 0.0)
         continue;

      if(tick.bid != g_last_bid[i] || tick.ask != g_last_ask[i])
         changed = true;
      g_last_bid[i] = tick.bid;
      g_last_ask[i] = tick.ask;

      int digits = (int)SymbolInfoInteger(g_symbols[i], SYMBOL_DIGITS);

      if(included > 0)
         ticks += ",";
      // tick.time is a 64-bit datetime; %d would truncate it, so render it
      // with IntegerToString instead.
      ticks += StringFormat("{\"symbol\":\"%s\",\"bid\":%s,\"ask\":%s,\"time\":%s}",
                            g_symbols[i],
                            DoubleToString(tick.bid, digits),
                            DoubleToString(tick.ask, digits),
                            IntegerToString((long)tick.time));
      included++;
     }

   if(included == 0)
      return;

   // Quiet markets (weekends, closed session) would otherwise mean a push per
   // second forever. The heartbeat still fires so the backend can tell "market
   // is flat" apart from "the terminal died".
   bool heartbeat_due = (InpHeartbeatSec > 0 && (now - g_last_push) >= InpHeartbeatSec);
   if(InpOnlyOnChange && !changed && !heartbeat_due)
      return;

   SendTicks("{\"ticks\":[" + ticks + "]}");
   g_last_push = now;
  }

//+------------------------------------------------------------------+
//| POST the JSON body to the backend.                               |
//+------------------------------------------------------------------+
void SendTicks(const string body)
  {
   string headers = "Content-Type: application/json\r\nX-MT5-Token: " + InpToken + "\r\n";

   uchar data[];
   int len = StringToCharArray(body, data, 0, WHOLE_ARRAY, CP_UTF8) - 1;
   if(len < 0)
      len = 0;
   ArrayResize(data, len);

   uchar  result[];
   string result_headers = "";

   ResetLastError();
   int status = WebRequest("POST", InpEndpoint, headers, InpTimeoutMs, data, result, result_headers);

   if(status == -1)
     {
      int err = GetLastError();
      if(err == 4014)
         PrintFormat("[Arbi] WebRequest blocked (error %d). Add '%s' under Tools -> Options -> Expert Advisors -> Allow WebRequest for listed URL.", err, OriginOf(InpEndpoint));
      else
         PrintFormat("[Arbi] WebRequest failed (error %d). Is the backend running and reachable at %s?", err, InpEndpoint);
      return;
     }

   if(status != 200)
      PrintFormat("[Arbi] Backend returned HTTP %d: %s", status, CharArrayToString(result, 0, WHOLE_ARRAY, CP_UTF8));
  }

//+------------------------------------------------------------------+
//| Strip the path off a URL, since the whitelist wants the origin.  |
//+------------------------------------------------------------------+
string OriginOf(const string url)
  {
   int scheme = StringFind(url, "://");
   if(scheme < 0)
      return(url);

   int slash = StringFind(url, "/", scheme + 3);
   if(slash < 0)
      return(url);

   return(StringSubstr(url, 0, slash));
  }
//+------------------------------------------------------------------+
