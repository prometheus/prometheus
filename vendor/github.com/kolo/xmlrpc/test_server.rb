# encoding: utf-8

require "xmlrpc/server"

class Service
  def time
    Time.now
  end

  def upcase(s)
    s.upcase
  end

  def sum(x, y)
    x + y
  end

  def error
    raise XMLRPC::FaultException.new(500, "Server error")
  end
end

server = XMLRPC::Server.new 5001, 'localhost'
server.add_handler "service", Service.new
server.serve
