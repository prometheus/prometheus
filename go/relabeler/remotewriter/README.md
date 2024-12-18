1. Создаем курсор.
2. Создаем клиент.
3. Цикл:
   1. создаем батч. (readTimeout)
      1. читаем следующий сегмент.
         1. если ошибка перманентная - батч завершен + блок завершен.
         2. если ошибка не перманентная - если батч полный или истекло время - батч завершен.
            1. пересчет количества шардов.
         3. если нет ошибок, но батч не полный, ждем 5 сек и повторяем п1.
   2. try go (write cache). (retry+backoff) + Close()
   3. encode protobuf.
   4. отправляем. (retry+backoff)
      1. если устарели
         1. return permanent error
      2. on error -> non permanent error
      3. on success -> nil
   5. try ack.
   6. check end of block
   