import sys
sys.path.insert(0, ".")
from lib import *
import part1_a as A, part1_b as B, part1_c as C

OUT = "/home/user/auctionEngine/backend-course/pdf/Part1-HTTP.pdf"
story = []
story += A.cover()
story += A.roadmap()
story += A.sec_big_picture()
story += A.sec_stateless()
story += A.sec_transport()
story += A.sec_message()
story += A.sec_headers()
story += B.sec_methods()
story += B.sec_cors()
story += B.sec_status()
story += B.sec_caching()
story += B.sec_negotiation()
story += B.sec_keepalive_large()
story += B.sec_tls()
story += C.sec_repo_map()
story += C.sec_layout()
story += C.sec_gaps()
story += [PageBreak()] + C.sec_qa()
story += C.sec_cheat()
Doc(OUT, "Backend from First Principles (Go) - Part 1: HTTP", "BACKEND FROM FIRST PRINCIPLES · GO  |  PART 1 · HTTP").build(story)
print("built", OUT)
