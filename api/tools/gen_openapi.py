"""Generates api/openapi.yaml.

Edit this script, not the YAML, then run from the repository root:

    python3 api/tools/gen_openapi.py api/openapi.yaml

hg-server's TestRoutesMatchOpenAPI checks the result against the registered routes,
and hg-studio regenerates its types from it (npm run gen:api).
"""
import sys
import yaml

OUT = sys.argv[1]

def ref(name): return {"$ref": f"#/components/schemas/{name}"}
def arr(item): return {"type": "array", "items": item}
def obj(props, required=None, desc=None):
    o = {"type": "object", "properties": props}
    if required: o["required"] = required
    if desc: o["description"] = desc
    return o
S = lambda d=None, **kw: {"type": "string", **({"description": d} if d else {}), **kw}
I = lambda d=None, **kw: {"type": "integer", **({"description": d} if d else {}), **kw}
B = lambda d=None: {"type": "boolean", **({"description": d} if d else {})}
UUID = {"type": "string", "format": "uuid"}
TIME = {"type": "string", "format": "date-time"}
def nullable(s): return {**s, "type": [s["type"], "null"]} if "type" in s else {"oneOf": [s, {"type": "null"}]}
WON = {"type": "integer", "format": "int64", "description": "Whole Korean won."}
def items_of(schema): return obj({"items": arr(schema)}, ["items"])

schemas = {
  "Error": obj({"error": S("Stable machine code, e.g. sold_out."), "message": S(), "details": {"description": "Optional structured details."}}, ["error"]),
  "Staff": obj({"id": UUID, "email": S(), "name": S(), "role": S(enum=["owner", "manager", "cs"]), "active": B(), "created_at": TIME}),
  "Customer": obj({"id": UUID, "email": S(), "name": S(), "created_at": TIME}),
  "Login": obj({"email": S(), "password": S()}, ["email", "password"]),
  "NewStaff": obj({"email": S(), "name": S(), "role": S(enum=["owner", "manager", "cs"]), "password": S(minLength=8, maxLength=72)}, ["email", "name", "role", "password"]),
  "StaffPatch": obj({"name": S(), "role": S(enum=["owner", "manager", "cs"]), "active": B(), "password": S()}),
  "NewCustomer": obj({"email": S(), "name": S(), "password": S(minLength=8, maxLength=72)}, ["email", "password"]),

  "OptionValue": obj({"id": UUID, "label": S(), "hex1": nullable(S("Swatch color #rrggbb.")), "hex2": nullable(S("Second color for two-tone swatches.")), "position": I()}),
  "Option": obj({"id": UUID, "name": S(), "position": I(), "values": arr(ref("OptionValue"))}),
  "Variant": obj({
      "id": UUID, "product_id": UUID, "option_value_ids": arr(UUID), "label": S("e.g. 'Brown / Mini'."),
      "position": I(), "price_override_won": nullable(WON), "price_won": {**WON, "description": "Effective price."},
      "stock_mode": S(enum=["counted", "until_sold_out"]), "stock_qty": I(), "held_qty": I(), "sold_qty": I(),
      "available_qty": nullable(I("null = unlimited (until_sold_out).")), "sold_out": B(),
      "status": S(enum=["active", "inactive"]), "buyable": B()}),
  "Media": obj({"id": UUID, "kind": S(enum=["photo", "inspection_photo", "inspection_video"]), "url": S(), "content_type": S(), "position": I()}),
  "Product": obj({
      "id": UUID, "title": S(), "description": S(), "price_won": WON,
      "status": S(enum=["draft", "live", "hidden", "archived"]), "source": S(enum=["live_capture", "manual"]),
      "specs": {"type": "object", "additionalProperties": {"type": "string"}}, "tags": arr(S()),
      "details_complete": B("False while the title is a placeholder (Live #n) or the description is empty."),
      "created_by": nullable(UUID), "created_at": TIME, "updated_at": TIME,
      "options": arr(ref("Option")), "variants": arr(ref("Variant")), "media": arr(ref("Media"))},
      desc="Staff view of a product, with stock bookkeeping."),
  "PublicVariant": obj({"id": UUID, "label": S(), "option_value_ids": arr(UUID), "price_won": WON,
      "available_qty": nullable(I()), "sold_out": B(), "buyable": B()}),
  "PublicProduct": obj({
      "id": UUID, "title": S(), "description": S(), "price_won": WON, "status": S(enum=["live", "hidden"]),
      "buyable": B(), "sold_out": B(), "specs": {"type": "object", "additionalProperties": {"type": "string"}},
      "tags": arr(S()), "options": arr(ref("Option")), "variants": arr(ref("PublicVariant")),
      "photos": arr(S("URL.")), "inspection": arr(obj({"kind": S(), "url": S()}))},
      desc="Storefront view of a product. There is no official (reference) price."),
  "NewOptionValue": obj({"label": S(maxLength=40), "hex1": S("Optional #rrggbb."), "hex2": S("Optional; needs hex1.")}, ["label"]),
  "NewOption": obj({"name": S(maxLength=30), "values": arr(ref("NewOptionValue"))}, ["name", "values"]),
  "NewStock": obj({"mode": S(enum=["counted", "until_sold_out"], default="counted"), "qty": I("Per variant.", minimum=0, maximum=100000)}),
  "NewProduct": obj({"title": S(maxLength=200), "description": S(), "price_won": WON,
      "status": S(enum=["draft", "live", "hidden", "archived"], default="draft"),
      "specs": {"type": "object", "additionalProperties": {"type": "string"}}, "tags": arr(S()),
      "options": arr(ref("NewOption")), "stock": ref("NewStock")}, ["title", "price_won"],
      desc="Every combination of option values becomes a variant (at most 100)."),
  "ProductPatch": obj({"title": S(), "description": S(), "price_won": WON, "status": S(enum=["draft", "live", "hidden", "archived"]),
      "specs": {"type": "object", "additionalProperties": {"type": "string"}}, "tags": arr(S())}),
  "VariantPatch": obj({"stock_mode": S(enum=["counted", "until_sold_out"]), "stock_qty": I(), "price_override_won": WON,
      "clear_price_override": B(), "sold_out": B(), "status": S(enum=["active", "inactive"])}),
  "Suggestion": obj({"label": S(), "hex1": nullable(S()), "hex2": nullable(S()), "uses": I()}),

  "Show": obj({"id": UUID, "title": S(), "youtube_video_id": S(), "status": S(enum=["draft", "scheduled", "live", "ended"]),
      "scheduled_at": nullable(TIME), "started_at": nullable(TIME), "ended_at": nullable(TIME),
      "display_days_after": I("Days items stay on sale after the show ends."), "next_number": I(),
      "host_name": S(), "created_at": TIME, "updated_at": TIME}),
  "NewShow": obj({"title": S(), "youtube_url": S("A video ID or a youtube.com / youtu.be link."), "scheduled_at": TIME,
      "display_days_after": I(default=3), "host_name": S(), "status": S(enum=["draft", "scheduled"], default="draft")}, ["title"]),
  "ShowPatch": obj({"title": S(), "youtube_url": S(), "scheduled_at": TIME, "display_days_after": I(), "host_name": S(),
      "status": S(enum=["draft", "scheduled"])}),
  "Item": obj({"id": UUID, "show_id": UUID, "number": I("Never reused or renumbered once the show is live."),
      "source": S(enum=["captured", "matched"], desc="captured: new product from a frame; matched: an existing product shown on air."), "product_id": UUID, "pinned": B("Worn or held today."),
      "capture_id": nullable(UUID), "broadcast_offset_sec": nullable(I()), "keep_listed": B(),
      "hide_at": nullable(TIME), "hidden_at": nullable(TIME), "published_at": TIME, "published_by": nullable(UUID)}),
  "ItemView": obj({"id": UUID, "number": I(), "source": S(enum=["captured", "matched"], desc="captured: new product from a frame; matched: an existing product shown on air."), "pinned": B(),
      "is_new": B("Listed on air in the last 15 minutes."), "published_at": TIME, "broadcast_offset_sec": nullable(I()),
      "replay_url": S("youtube.com/watch?v={id}&t={offset}s, or empty."), "product": ref("PublicProduct")}),
  "StudioItem": {"allOf": [ref("Item"), obj({"replay_url": S(), "product": ref("Product")})]},
  "ShowView": {"allOf": [ref("Show"), obj({"watch_url": S(), "items": arr(ref("ItemView")),
      "last_event_id": I("Open /events from this id to miss nothing.", format="int64")})]},
  "StudioShow": {"allOf": [ref("Show"), obj({"watch_url": S(), "items": arr(ref("StudioItem")), "last_event_id": I(format="int64")})]},
  "Capture": obj({"id": UUID, "show_id": UUID, "content_type": S(), "width": I(), "height": I(),
      "broadcast_offset_sec": nullable(I()), "source": S(enum=["upload", "relay"]), "created_by": nullable(UUID),
      "created_at": TIME, "image_url": S("Staff-only image route.")}),
  "Crop": obj({"x": I(), "y": I(), "w": I(minimum=16), "h": I(minimum=16)}, ["x", "y", "w", "h"]),
  "QuickListInput": obj({
      "capture_id": UUID, "crop": ref("Crop"), "product_id": UUID, "title": S("Defaults to 'Live #<number>'."),
      "description": S(), "price_won": WON, "options": arr(ref("NewOption")), "stock": ref("NewStock")},
      desc="New product: capture_id and price_won are required. Existing product: product_id, and optionally price_won to change its price."),
  "Broadcast": obj({"show_id": UUID, "show_title": S(), "number": I(), "shown_at": nullable(TIME),
      "broadcast_offset_sec": nullable(I()), "replay_url": S()}),

  "Ingest": obj({"url": S("Full SRT URL with passphrase; paste into OBS as Server."), "host": S(), "passphrase": S(),
      "latency_ms": I(), "note": S()}),
  "RelayStatus": obj({"show_id": UUID, "enabled": B(), "listening": B("ffmpeg is waiting for or receiving the broadcast."),
      "receiving": B("A frame arrived in the last 3 seconds."), "ingest_started_at": nullable(TIME),
      "last_frame_at": nullable(TIME), "frames_buffered": I(), "sessions": I("Broadcaster connections so far."),
      "youtube_push": B("A YouTube stream key is configured."), "push_running": B(), "push_restarts": I(),
      "push_last_error": S(), "last_error": S()}),
  "FrameInfo": obj({"seq": I(format="int64"), "at": TIME, "offset_sec": nullable(I("Place in the stream, from ingest start.")), "url": S()}),
  "Shipping": obj({"recipient": S(), "phone": S(), "postcode": S(), "address1": S(), "address2": S(),
      "country": S(default="KR"), "pccc": S("Korea personal customs clearance code: P + 12 digits. Required before shipping.")},
      ["recipient", "phone", "postcode", "address1"]),
  "Line": obj({"id": UUID, "product_id": UUID, "variant_id": UUID, "title": S(), "variant_label": S(), "photo_url": S(),
      "unit_price_won": WON, "qty": I()}),
  "OrderMedia": obj({"id": UUID, "kind": S(enum=["photo", "video"]), "content_type": S(), "url": S(), "created_at": TIME}),
  "History": obj({"from": S(), "to": S(), "actor_kind": S(enum=["customer", "staff", "system", "payment"]),
      "actor_id": nullable(UUID), "note": S(), "at": TIME}),
  "OrderCounts": obj({st: I() for st in ["pending_payment", "paid", "inspecting", "customs_check", "shipped",
                                             "delivered", "completed", "canceled"]}, desc="Orders per status."),
  "ItemSales": obj({"item_id": UUID, "orders": I(), "units": I(), "sales_won": WON}),
  "ShowSales": obj({"show_id": UUID, "orders": I("Paid orders that are not canceled."), "units": I(), "sales_won": WON,
      "pending_orders": I("Waiting for payment."), "canceled_orders": I("Paid, then canceled and refunded."),
      "items": arr(ref("ItemSales"))},
      desc="Orders from one stream. An order line counts for a stream when its product is one of the stream's items, "
           "the order was placed after the stream started, and no later stream with the same product had started. "
           "Amounts are line totals without shipping. Items without orders are left out."),
  "Order": obj({
      "id": UUID, "number": S("HG-YYMMDD-XXXXX: the order date in Korea time and five random characters "
                                          "(digits 2-9 and consonants), e.g. HG-261107-7KQ4M."), "customer_id": UUID,
      "status": S(enum=["pending_payment", "paid", "inspecting", "customs_check", "shipped", "delivered", "completed", "canceled"]),
      "subtotal_won": WON, "shipping_fee_won": WON, "total_won": WON, "refunded_won": WON, "shipping": ref("Shipping"),
      "payment_method": S(enum=["card", "virtual_account"]), "payment_deadline": TIME, "paid_at": nullable(TIME),
      "inspection_done_at": nullable(TIME), "customs_checked_at": nullable(TIME), "shipped_at": nullable(TIME),
      "carrier": S(), "tracking_number": S(), "delivered_at": nullable(TIME), "completed_at": nullable(TIME),
      "canceled_at": nullable(TIME), "cancel_reason": S(), "created_at": TIME, "updated_at": TIME,
      "lines": arr(ref("Line")), "inspection_media": arr(ref("OrderMedia")),
      "history": arr(ref("History"))},
      desc="Customers see inspection_media once inspection is done; history is staff-only."),
  "Checkout": obj({"id": UUID, "customer_id": UUID, "status": S(enum=["open", "ordered", "expired", "canceled"]),
      "expires_at": TIME, "shipping": nullable(ref("Shipping")), "order_id": nullable(UUID), "lines": arr(ref("Line")),
      "subtotal_won": WON, "shipping_fee_won": WON, "total_won": WON, "created_at": TIME}),
  "VirtualAccount": obj({"bank": S(), "account": S(), "holder": S(), "due_at": TIME}),
  "Payment": obj({"id": UUID, "order_id": UUID, "provider": S(), "method": S(enum=["card", "virtual_account"]),
      "status": S(enum=["pending", "succeeded", "failed", "canceled"]), "amount_won": WON, "refunded_won": WON,
      "provider_ref": S(), "redirect_url": S("Where to send the customer to pay by card."),
      "virtual_account": nullable(ref("VirtualAccount")), "failure_reason": S(), "succeeded_at": nullable(TIME), "created_at": TIME}),
  "PayResult": obj({"order": ref("Order"), "payment": ref("Payment")}),
}

# Response objects always carry every field (Go encodes them all), so mark
# them required; request bodies keep their own required lists.
REQUEST_ONLY = {"Login", "NewStaff", "StaffPatch", "NewCustomer", "NewOptionValue", "NewOption", "NewStock",
                "NewProduct", "ProductPatch", "VariantPatch", "NewShow", "ShowPatch", "Crop", "QuickListInput",
                "Shipping", "Error"}
def _all_required(sch):
    if isinstance(sch, dict):
        if sch.get("type") == "object" and "properties" in sch:
            sch["required"] = list(sch["properties"].keys())
        for v in sch.values():
            if isinstance(v, (dict, list)):
                _all_required(v)
    elif isinstance(sch, list):
        for v in sch:
            _all_required(v)
for _name, _sch in schemas.items():
    if _name not in REQUEST_ONLY:
        _all_required(_sch)
# Order.history is only in the staff detail (GET /studio-api/orders/{id} and transitions), not in lists.
schemas["Order"]["required"].remove("history")
schemas["Order"]["properties"]["history"]["description"] = "Staff order detail only."

def resp(schema=None, desc="OK", status="200"):
    r = {"description": desc}
    if schema is not None:
        r["content"] = {"application/json": {"schema": schema}}
    return {status: r}

ERR = {"description": "Error", "content": {"application/json": {"schema": ref("Error")}}}
SSE = {"200": {"description": "Server-Sent Events stream. Each event: `id`, `event` (type) and `data` (JSON). "
                              "Resume with Last-Event-ID or ?last_event_id=. `event: reset` means reload the snapshot.",
               "content": {"text/event-stream": {"schema": {"type": "string"}}}}}
def body(schema, ctype="application/json"):
    return {"required": True, "content": {ctype: {"schema": schema}}}
def upload_body(fields):
    props = {"file": {"type": "string", "format": "binary"}, **fields}
    return body(obj(props, ["file"]), "multipart/form-data")

P = {  # path parameters
  "id": {"name": "id", "in": "path", "required": True, "schema": UUID},
  "variant_id": {"name": "variant_id", "in": "path", "required": True, "schema": UUID},
  "media_id": {"name": "media_id", "in": "path", "required": True, "schema": UUID},
  "item_id": {"name": "item_id", "in": "path", "required": True, "schema": UUID},
  "seq": {"name": "seq", "in": "path", "required": True, "schema": I(format="int64")},
  "provider": {"name": "provider", "in": "path", "required": True, "schema": S()},
  "path": {"name": "path", "in": "path", "required": True, "schema": S(), "description": "Object path, may contain slashes."},
}
def q(name, schema, desc=None):
    p = {"name": name, "in": "query", "schema": schema}
    if desc: p["description"] = desc
    return p

STAFF = [{"staffSession": []}]
CUST = [{"customerSession": []}]
def role(r): return f" Role: {r} or higher."

paths = {}
def op(method, path, tag, summary, responses, security=None, params=None, request=None, desc=None):
    entry = paths.setdefault(path, {})
    o = {"tags": [tag], "summary": summary, "responses": {**responses, "default": ERR}}
    if desc: o["description"] = desc
    if security is not None: o["security"] = security
    if params: o["parameters"] = params
    if request: o["requestBody"] = request
    entry[method.lower()] = o
    # path-level params
    names = [seg[1:-1].replace("...", "") for seg in path.split("/") if seg.startswith("{")]
    if names:
        entry["parameters"] = [P[n] for n in names]

pg = [q("limit", I()), q("offset", I())]

# --- Studio: auth & staff
op("POST", "/studio-api/auth/login", "studio-auth", "Staff sign-in (sets hg_staff_session)", resp(ref("Staff")), [], request=body(ref("Login")))
op("POST", "/studio-api/auth/logout", "studio-auth", "Staff sign-out", resp(desc="Signed out", status="204"), [])
op("GET", "/studio-api/auth/me", "studio-auth", "Current staff member", resp(ref("Staff")), STAFF)
op("GET", "/studio-api/staff", "studio-auth", "List staff." + role("owner"), resp(items_of(ref("Staff"))), STAFF)
op("POST", "/studio-api/staff", "studio-auth", "Add a staff member." + role("owner"), resp(ref("Staff"), "Created", "201"), STAFF, request=body(ref("NewStaff")))
op("PATCH", "/studio-api/staff/{id}", "studio-auth", "Change a staff member." + role("owner"), resp(ref("Staff")), STAFF, request=body(ref("StaffPatch")),
   desc="Deactivating or changing the password ends their sessions. The last active owner cannot be demoted.")

# --- Storefront: auth
op("POST", "/api/auth/register", "auth", "Register (sets hg_session)", resp(ref("Customer"), "Created", "201"), [], request=body(ref("NewCustomer")))
op("POST", "/api/auth/login", "auth", "Sign in (sets hg_session)", resp(ref("Customer")), [], request=body(ref("Login")))
op("POST", "/api/auth/logout", "auth", "Sign out", resp(desc="Signed out", status="204"), [])
op("GET", "/api/me", "auth", "Current customer", resp(ref("Customer")), CUST)

# --- Studio: catalog
op("GET", "/studio-api/products", "studio-catalog", "List products." + role("cs"), resp(items_of(ref("Product"))), STAFF,
   params=[q("status", S(), "Comma-separated statuses."), q("q", S(), "Title contains."), q("incomplete", B(), "Only products missing details."), *pg])
op("POST", "/studio-api/products", "studio-catalog", "Create a product by hand." + role("cs"), resp(ref("Product"), "Created", "201"), STAFF, request=body(ref("NewProduct")))
op("GET", "/studio-api/products/{id}", "studio-catalog", "Get a product." + role("cs"), resp(ref("Product")), STAFF)
op("PATCH", "/studio-api/products/{id}", "studio-catalog", "Change product fields." + role("cs"), resp(ref("Product")), STAFF, request=body(ref("ProductPatch")))
op("PATCH", "/studio-api/products/{id}/variants/{variant_id}", "studio-catalog", "Change stock, price override, sold-out or status." + role("cs"),
   resp(ref("Product")), STAFF, request=body(ref("VariantPatch")), desc="409 stock_below_committed when stock would fall below held + sold.")
op("POST", "/studio-api/products/{id}/sold-out", "studio-catalog", "Mark every variant sold out." + role("cs"), resp(ref("Product")), STAFF)
op("POST", "/studio-api/products/{id}/media", "studio-catalog", "Upload a photo or inspection photo/video." + role("cs"),
   resp(ref("Media"), "Created", "201"), STAFF, request=upload_body({"kind": S(enum=["photo", "inspection_photo", "inspection_video"], default="photo")}),
   desc="Images are re-encoded as JPEG (metadata stripped), up to 15 MB. Videos: MP4, QuickTime or WebM, up to 200 MB.")
op("DELETE", "/studio-api/products/{id}/media/{media_id}", "studio-catalog", "Remove a photo or inspection file." + role("cs"), resp(desc="Removed", status="204"), STAFF)
op("GET", "/studio-api/catalog/suggestions", "studio-catalog", "Recently used option values." + role("cs"), resp(items_of(ref("Suggestion"))), STAFF,
   params=[{**q("option", S(), "Option name, e.g. Color (case-insensitive)."), "required": True}],
   desc="The labels used most in the last 90 days, with their swatch colors, so 'Brown' keeps one spelling.")

# --- Storefront: catalog
op("GET", "/api/products", "catalog", "List live products", resp(items_of(ref("PublicProduct"))), [], params=[q("q", S()), *pg])
op("GET", "/api/products/{id}", "catalog", "Product page", resp(ref("PublicProduct")), [],
   desc="Live and hidden products are returned (hidden ones cannot be bought); drafts and archived ones are 404.")

# --- Studio: live
op("GET", "/studio-api/shows", "studio-live", "List shows." + role("cs"), resp(items_of(ref("Show"))), STAFF, params=[q("status", S(), "Comma-separated."), q("limit", I())])
op("POST", "/studio-api/shows", "studio-live", "Create a show." + role("manager"), resp(ref("Show"), "Created", "201"), STAFF, request=body(ref("NewShow")))
op("GET", "/studio-api/shows/{id}", "studio-live", "Show with items and full products." + role("cs"), resp(ref("StudioShow")), STAFF)
op("PATCH", "/studio-api/shows/{id}", "studio-live", "Change a show." + role("manager"), resp(ref("Show")), STAFF, request=body(ref("ShowPatch")))
op("POST", "/studio-api/shows/{id}/start", "studio-live", "Go live." + role("cs"), resp(ref("Show")), STAFF,
   desc="The show starts with no items: every product is listed on air, numbered from No.1. 409 another_show_live.")
op("POST", "/studio-api/shows/{id}/end", "studio-live", "End the show." + role("cs"), resp(ref("Show")), STAFF,
   desc="Items not kept listed are hidden display_days_after days later.")
op("GET", "/studio-api/shows/{id}/events", "studio-live", "Show event stream (staff)." + role("cs"), SSE, STAFF)
op("POST", "/studio-api/shows/{id}/captures", "studio-live", "Upload a captured frame." + role("cs"), resp(ref("Capture"), "Created", "201"), STAFF,
   request=upload_body({}), desc="Stored unchanged (JPEG or PNG). broadcast_offset_sec is set while the show is live.")
op("GET", "/studio-api/captures/{id}", "studio-live", "Capture details." + role("cs"), resp(ref("Capture")), STAFF)
op("GET", "/studio-api/captures/{id}/image", "studio-live", "Capture image." + role("cs"),
   {"200": {"description": "The image", "content": {"image/jpeg": {}, "image/png": {}}}}, STAFF)
op("POST", "/studio-api/shows/{id}/items/quick", "studio-live", "Quick listing: publish an item on air." + role("cs"),
   resp(ref("StudioItem"), "Created", "201"), STAFF, request=body(ref("QuickListInput")),
   params=[{"name": "Idempotency-Key", "in": "header", "schema": S(maxLength=100), "description": "A retry with the same key returns the same item."}],
   desc="One transaction: product, options, variants, stock, photo, numbered item and the viewer event commit together or not at all. "
        "409 show_not_live, already_in_show.")
op("POST", "/studio-api/items/{id}/pin", "studio-live", "Pin as worn or held today." + role("cs"), resp(ref("StudioItem")), STAFF)
op("POST", "/studio-api/items/{id}/unpin", "studio-live", "Unpin." + role("cs"), resp(ref("StudioItem")), STAFF)
op("POST", "/studio-api/items/{id}/sold-out", "studio-live", "Mark the item's product sold out." + role("cs"), resp(ref("StudioItem")), STAFF)
op("PATCH", "/studio-api/items/{id}", "studio-live", "Keep an item listed after the stream (or not)." + role("manager"), resp(ref("StudioItem")), STAFF,
   request=body(obj({"keep_listed": B()}, ["keep_listed"])))

# --- Storefront: live
op("GET", "/api/shows/current", "live", "The live show, else the next scheduled one", resp(obj({"show": nullable(ref("ShowView"))})), [],
   desc="Items are listed once the show is live.")
op("GET", "/api/shows", "live", "Ended shows (recaps)", resp(items_of(ref("Show"))), [], params=[q("status", S(enum=["ended"])), q("limit", I())])
op("GET", "/api/shows/{id}", "live", "Live page snapshot", resp(ref("ShowView")), [])
op("GET", "/api/shows/{id}/events", "live", "Live page event stream", SSE, [],
   desc="Event types: live.show_started, live.show_ended, live.show_updated, live.item_published, live.item_updated "
        "(price, title, photos, stock, sold out), live.item_pinned, live.item_unpinned. "
        "Item events carry the full ItemView in data.item.")
op("GET", "/api/shows/{id}/recap", "live", "Recap of an ended show", resp(ref("ShowView")), [], desc="409 show_not_ended before the show ends.")
op("GET", "/api/products/{id}/broadcasts", "live", "Where a product was shown, with replay links", resp(items_of(ref("Broadcast"))), [])

# --- Studio: broadcast relay
op("GET", "/studio-api/shows/{id}/ingest", "studio-relay", "Ingest URL and passphrase for OBS." + role("manager"), resp(ref("Ingest")), STAFF,
   desc="409 relay_disabled when HG_RELAY_ENABLED is false.")
op("POST", "/studio-api/shows/{id}/ingest/rotate", "studio-relay", "Replace the ingest passphrase." + role("manager"), resp(ref("Ingest")), STAFF,
   desc="A live relay restarts with the new passphrase; the broadcaster must reconnect.")
op("GET", "/studio-api/shows/{id}/relay", "studio-relay", "Relay status." + role("cs"), resp(ref("RelayStatus")), STAFF)
op("GET", "/studio-api/shows/{id}/frames", "studio-relay", "Recent frames from the ring buffer (last 120 s)." + role("cs"),
   resp(items_of(ref("FrameInfo"))), STAFF, params=[q("after_seq", I(format="int64"), "Frames after this one; omit for the newest."), q("limit", I())],
   desc="409 no_frames before the relay has received the show's stream.")
op("GET", "/studio-api/shows/{id}/frames/{seq}", "studio-relay", "One buffered frame." + role("cs"),
   {"200": {"description": "The frame", "content": {"image/jpeg": {}}}}, STAFF)
op("GET", "/studio-api/shows/{id}/preview", "studio-relay", "Live preview as MJPEG." + role("cs"),
   {"200": {"description": "multipart/x-mixed-replace stream of JPEG frames; show it in an <img> element.",
            "content": {"multipart/x-mixed-replace": {"schema": {"type": "string"}}}}}, STAFF)
op("POST", "/studio-api/shows/{id}/captures/relay", "studio-relay", "Keep a buffered frame as a capture." + role("cs"),
   resp(ref("Capture"), "Created", "201"), STAFF, request=body(obj({"seq": I(format="int64")}, ["seq"])),
   desc="The capture's broadcast_offset_sec is the frame's place in the stream.")

# --- Studio: orders
op("GET", "/studio-api/orders", "studio-orders", "List orders, newest first." + role("cs"), resp(items_of(ref("Order"))), STAFF,
   params=[q("status", S(), "Comma-separated."),
           q("q", S(maxLength=100), "Order number or recipient (part of it), or 3+ digits of the phone number."), *pg])
op("GET", "/studio-api/orders/counts", "studio-orders", "Number of orders in each status." + role("cs"),
   resp(obj({"counts": ref("OrderCounts")})), STAFF)
op("GET", "/studio-api/shows/{id}/sales", "studio-orders", "Orders and sales from one stream." + role("cs"), resp(ref("ShowSales")), STAFF)
op("GET", "/studio-api/orders/events", "studio-orders", "Order event stream (order.created, order.updated)." + role("cs"), SSE, STAFF)
op("GET", "/studio-api/orders/{id}", "studio-orders", "Order with history." + role("cs"), resp(ref("Order")), STAFF)
op("GET", "/studio-api/orders/{id}/media/{media_id}", "studio-orders", "Inspection file." + role("cs"), {"200": {"description": "The file"}}, STAFF)
op("POST", "/studio-api/orders/{id}/inspection", "studio-orders", "Add an inspection photo or video." + role("cs"), resp(ref("Order"), "Created", "201"), STAFF,
   request=upload_body({"kind": S(enum=["photo", "video"], default="photo")}), desc="The first file moves a paid order to inspecting.")
op("POST", "/studio-api/orders/{id}/inspection/complete", "studio-orders", "Finish inspection (inspecting -> customs_check)." + role("cs"), resp(ref("Order")), STAFF,
   request={"required": False, "content": {"application/json": {"schema": obj({"note": S()})}}})
op("POST", "/studio-api/orders/{id}/customs-check", "studio-orders", "Record the customs code check." + role("cs"), resp(ref("Order")), STAFF,
   desc="409 pccc_missing when the order has no valid customs code.")
op("POST", "/studio-api/orders/{id}/ship", "studio-orders", "Ship (customs_check -> shipped)." + role("cs"), resp(ref("Order")), STAFF,
   request=body(obj({"carrier": S(), "tracking_number": S()}, ["carrier", "tracking_number"])), desc="409 customs_not_checked.")
op("POST", "/studio-api/orders/{id}/deliver", "studio-orders", "Mark delivered." + role("cs"), resp(ref("Order")), STAFF)
op("POST", "/studio-api/orders/{id}/complete", "studio-orders", "Close a delivered order." + role("cs"), resp(ref("Order")), STAFF)
op("POST", "/studio-api/orders/{id}/cancel", "studio-orders", "Cancel and refund." + role("manager"), resp(ref("Order")), STAFF,
   request={"required": False, "content": {"application/json": {"schema": obj({"reason": S()})}}},
   desc="Refunds through the provider first. Stock returns if the order had not shipped.")

# --- Storefront: orders, checkout
op("GET", "/api/orders", "orders", "My orders", resp(items_of(ref("Order"))), CUST, params=[q("limit", I())])
op("GET", "/api/orders/{id}", "orders", "My order", resp(ref("Order")), CUST)
op("GET", "/api/orders/{id}/media/{media_id}", "orders", "Inspection file of my order", {"200": {"description": "The file"}}, CUST)
op("POST", "/api/orders/{id}/cancel", "orders", "Cancel my order (while unpaid or paid)", resp(ref("Order")), CUST,
   request={"required": False, "content": {"application/json": {"schema": obj({"reason": S()})}}})
op("POST", "/api/orders/{id}/receipt", "orders", "Confirm receipt (delivered -> completed)", resp(ref("Order")), CUST)
op("PUT", "/api/orders/{id}/customs-code", "orders", "Correct the customs code before it is checked", resp(ref("Order")), CUST,
   request=body(obj({"pccc": S()}, ["pccc"])))
op("POST", "/api/orders/{id}/pay", "checkout", "Retry payment for a pending order", resp(ref("PayResult"), "Created", "201"), CUST)
op("POST", "/api/checkout", "checkout", "Buy now: hold stock and open a checkout", resp(ref("Checkout"), "Created", "201"), CUST,
   request=body(obj({"lines": arr(obj({"variant_id": UUID, "qty": I(minimum=1, maximum=100)}, ["variant_id", "qty"]))}, ["lines"])),
   desc="Holds the stock until expires_at (10 minutes by default). 409 sold_out when it is gone.")
op("GET", "/api/checkout/{id}", "checkout", "My checkout", resp(ref("Checkout")), CUST)
op("PUT", "/api/checkout/{id}/shipping", "checkout", "Set address and customs code", resp(ref("Checkout")), CUST, request=body(ref("Shipping")),
   desc="409 checkout_expired once the hold has run out.")
op("POST", "/api/checkout/{id}/pay", "checkout", "Place the order and start the payment", resp(ref("PayResult"), "Created", "201"), CUST,
   request=body(obj({"method": S(enum=["card", "virtual_account"])}, ["method"])),
   desc="Card: send the customer to payment.redirect_url. Virtual account: show payment.virtual_account. 502 payment_provider_unavailable; retry with /api/orders/{id}/pay.")
op("DELETE", "/api/checkout/{id}", "checkout", "Abandon a checkout and release its holds", resp(desc="Canceled", status="204"), CUST)

# --- Hooks, dev, media, health
op("POST", "/hooks/payments/{provider}", "hooks", "Payment provider webhook", resp(obj({"ok": B()})), [],
   request=body({"type": "object"}), desc="Signature-checked by the provider adapter; repeated events are ignored.")
op("POST", "/api/dev/payments/{id}/approve", "dev", "Development only: approve a fake payment", resp(ref("Payment")), [],
   desc="Exists only with HG_DEV_ROUTES=true and the fake provider. Sends a signed webhook as the provider would.")
op("POST", "/api/dev/payments/{id}/fail", "dev", "Development only: decline a fake payment", resp(ref("Payment")), [])
op("GET", "/media/{path}", "media", "Public media (product photos and product inspection files)",
   {"200": {"description": "The file, cacheable for a year"}}, [])
op("GET", "/healthz", "health", "Health check", resp(obj({"status": S()})), [])

doc = {
  "openapi": "3.1.0",
  "info": {
    "title": "hg-server API", "version": "0.1.0",
    "description": (
      "The HTTP contract between hg-server and its two apps.\n\n"
      "- **/api**: used by storefront. Customer session cookie `hg_session`.\n"
      "- **/studio-api**: used by hg-studio (H-Global Studio). Staff session cookie `hg_staff_session`; "
      "roles cs < manager < owner.\n"
      "- **/hooks**: payment provider webhooks.\n\n"
      "Money is whole Korean won (`*_won`). Times are RFC 3339. IDs are UUIDv7. Errors are "
      "`{\"error\": code, \"message\": text}` with 400, 401, 403, 404, 409 or 502. "
      "State-changing requests with an `Origin` header outside HG_ALLOWED_ORIGINS get 403 origin_not_allowed.\n\n"
      "hg-server's tests fail when a registered route is missing here (internal/app/openapi_test.go)."),
  },
  "servers": [{"url": "http://localhost:8080", "description": "Local hg-server"}],
  "tags": [{"name": n} for n in ["auth", "catalog", "live", "checkout", "orders", "studio-auth", "studio-catalog",
                                   "studio-live", "studio-relay", "studio-orders", "hooks", "dev", "media", "health"]],
  "paths": paths,
  "components": {
    "securitySchemes": {
      "customerSession": {"type": "apiKey", "in": "cookie", "name": "hg_session"},
      "staffSession": {"type": "apiKey", "in": "cookie", "name": "hg_staff_session"},
    },
    "schemas": schemas,
  },
}

class NoAlias(yaml.SafeDumper):
    def ignore_aliases(self, data): return True

with open(OUT, "w") as f:
    f.write("# hg-server HTTP API. Generated by api/tools/gen_openapi.py: edit that script, not this file.\n")
    yaml.dump(doc, f, Dumper=NoAlias, sort_keys=False, allow_unicode=True, width=120)
print("paths:", len(paths), "operations:", sum(len([k for k in v if k != 'parameters']) for v in paths.values()))
